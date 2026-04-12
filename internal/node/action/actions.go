package action

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/node"
	"github.com/FlameInTheDark/emerald/internal/proxmox"
	"github.com/FlameInTheDark/emerald/internal/shellcmd"
	"github.com/FlameInTheDark/emerald/internal/templating"
)

type ClusterStore interface {
	GetByID(ctx context.Context, id string) (*models.Cluster, error)
}

type ChannelStore interface {
	GetByID(ctx context.Context, id string) (*models.Channel, error)
}

type ChannelContactStore interface {
	GetByID(ctx context.Context, id string) (*models.ChannelContact, error)
}

type ChannelMessageSender interface {
	SendMessage(ctx context.Context, channel *models.Channel, chatID string, text string) (map[string]any, error)
	ReplyMessage(ctx context.Context, channel *models.Channel, chatID string, replyToMessageID string, text string) (map[string]any, error)
	EditMessage(ctx context.Context, channel *models.Channel, chatID string, messageID string, text string) (map[string]any, error)
}

type ChannelReplyWaiter interface {
	WaitForReply(ctx context.Context, channelID string, contactID string, chatID string, sentMessageID string, timeout time.Duration) (map[string]any, error)
}

type proxmoxConfig struct {
	ClusterID string `json:"clusterId"`
}

func resolveClusterID(configClusterID string, input map[string]any) string {
	if configClusterID != "" {
		return configClusterID
	}
	if clusterID, ok := input["clusterId"].(string); ok && clusterID != "" {
		return clusterID
	}
	if clusterID, ok := input["cluster_id"].(string); ok && clusterID != "" {
		return clusterID
	}
	return ""
}

func resolveChannelID(configChannelID string, input map[string]any) string {
	if value := strings.TrimSpace(configChannelID); value != "" {
		return value
	}
	if value, ok := input["channel_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := input["channelId"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

func loadClusterClient(ctx context.Context, store ClusterStore, clusterID string, input map[string]any) (*proxmox.Client, *models.Cluster, error) {
	if store == nil {
		return nil, nil, fmt.Errorf("cluster store is not configured")
	}

	resolvedClusterID := resolveClusterID(clusterID, input)
	if resolvedClusterID == "" {
		return nil, nil, fmt.Errorf("clusterId is required")
	}

	cluster, err := store.GetByID(ctx, resolvedClusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("load cluster %s: %w", resolvedClusterID, err)
	}

	client := proxmox.NewClient(proxmox.ClientConfig{
		Host:          cluster.Host,
		Port:          cluster.Port,
		TokenID:       cluster.APITokenID,
		TokenSecret:   cluster.APITokenSecret,
		SkipTLSVerify: cluster.SkipTLSVerify,
	})

	return client, cluster, nil
}

type ListNodesAction struct {
	Clusters ClusterStore
}

func (e *ListNodesAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg proxmoxConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	client, cluster, err := loadClusterClient(ctx, e.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes for cluster %s: %w", cluster.Name, err)
	}

	output := map[string]any{
		"clusterId":   cluster.ID,
		"clusterName": cluster.Name,
		"count":       len(nodes),
		"nodes":       nodes,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ListNodesAction) Validate(config json.RawMessage) error {
	var cfg proxmoxConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("clusterId is required")
	}
	return nil
}

type ListVMsCTsAction struct {
	Clusters ClusterStore
}

type listVMsCTsConfig struct {
	ClusterID string `json:"clusterId"`
	Node      string `json:"node"`
}

func (e *ListVMsCTsAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg listVMsCTsConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Node == "" {
		if nodeName, ok := input["node"].(string); ok {
			cfg.Node = nodeName
		}
	}

	client, cluster, err := loadClusterClient(ctx, e.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	resources, err := client.ListClusterResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workloads for cluster %s: %w", cluster.Name, err)
	}

	vms := make([]map[string]any, 0)
	containers := make([]map[string]any, 0)
	workloads := make([]map[string]any, 0)

	for _, resource := range resources {
		resourceType, _ := resource["type"].(string)
		if resourceType != "qemu" && resourceType != "lxc" {
			continue
		}

		if cfg.Node != "" {
			resourceNode, _ := resource["node"].(string)
			if resourceNode != cfg.Node {
				continue
			}
		}

		workload := resource
		workloads = append(workloads, workload)

		if resourceType == "qemu" {
			vms = append(vms, workload)
			continue
		}
		containers = append(containers, workload)
	}

	output := map[string]any{
		"clusterId":      cluster.ID,
		"clusterName":    cluster.Name,
		"node":           cfg.Node,
		"count":          len(workloads),
		"vmCount":        len(vms),
		"containerCount": len(containers),
		"workloads":      workloads,
		"vms":            vms,
		"containers":     containers,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ListVMsCTsAction) Validate(config json.RawMessage) error {
	var cfg listVMsCTsConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("clusterId is required")
	}
	return nil
}

type VMStartAction struct {
	Clusters ClusterStore
}

type vmStartConfig struct {
	ClusterID string `json:"clusterId"`
	Node      string `json:"node"`
	VMID      int    `json:"vmid"`
}

func (e *VMStartAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Node == "" {
		if n, ok := input["node"].(string); ok {
			cfg.Node = n
		}
	}
	if cfg.VMID == 0 {
		if v, ok := input["vmid"].(float64); ok {
			cfg.VMID = int(v)
		}
	}

	if cfg.Node == "" || cfg.VMID == 0 {
		return nil, fmt.Errorf("node and vmid are required")
	}

	client, cluster, err := loadClusterClient(ctx, e.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	if err := client.StartVM(ctx, cfg.Node, cfg.VMID); err != nil {
		return nil, fmt.Errorf("start vm %d on node %s: %w", cfg.VMID, cfg.Node, err)
	}

	output := map[string]any{
		"status":      "started",
		"clusterId":   cluster.ID,
		"clusterName": cluster.Name,
		"node":        cfg.Node,
		"vmid":        cfg.VMID,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *VMStartAction) Validate(config json.RawMessage) error {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Node == "" {
		return fmt.Errorf("node is required")
	}
	if cfg.VMID == 0 {
		return fmt.Errorf("vmid is required")
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("clusterId is required")
	}
	return nil
}

type VMStopAction struct {
	Clusters ClusterStore
}

func (e *VMStopAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Node == "" {
		if n, ok := input["node"].(string); ok {
			cfg.Node = n
		}
	}
	if cfg.VMID == 0 {
		if v, ok := input["vmid"].(float64); ok {
			cfg.VMID = int(v)
		}
	}

	if cfg.Node == "" || cfg.VMID == 0 {
		return nil, fmt.Errorf("node and vmid are required")
	}

	client, cluster, err := loadClusterClient(ctx, e.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	if err := client.StopVM(ctx, cfg.Node, cfg.VMID); err != nil {
		return nil, fmt.Errorf("stop vm %d on node %s: %w", cfg.VMID, cfg.Node, err)
	}

	output := map[string]any{
		"status":      "stopped",
		"clusterId":   cluster.ID,
		"clusterName": cluster.Name,
		"node":        cfg.Node,
		"vmid":        cfg.VMID,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *VMStopAction) Validate(config json.RawMessage) error {
	var cfg vmStartConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Node == "" {
		return fmt.Errorf("node is required")
	}
	if cfg.VMID == 0 {
		return fmt.Errorf("vmid is required")
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("clusterId is required")
	}
	return nil
}

type VMCloneAction struct {
	Clusters ClusterStore
}

type vmCloneConfig struct {
	ClusterID string `json:"clusterId"`
	Node      string `json:"node"`
	VMID      int    `json:"vmid"`
	NewName   string `json:"newName"`
	NewID     int    `json:"newId"`
}

func (e *VMCloneAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg vmCloneConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Node == "" {
		if n, ok := input["node"].(string); ok {
			cfg.Node = n
		}
	}
	if cfg.VMID == 0 {
		if v, ok := input["vmid"].(float64); ok {
			cfg.VMID = int(v)
		}
	}
	if cfg.NewID == 0 {
		if v, ok := input["newId"].(float64); ok {
			cfg.NewID = int(v)
		}
	}
	if cfg.NewName == "" {
		if n, ok := input["newName"].(string); ok {
			cfg.NewName = n
		}
	}

	if cfg.Node == "" || cfg.VMID == 0 || cfg.NewName == "" || cfg.NewID == 0 {
		return nil, fmt.Errorf("node, vmid, newName, and newId are required")
	}

	client, cluster, err := loadClusterClient(ctx, e.Clusters, cfg.ClusterID, input)
	if err != nil {
		return nil, err
	}

	if err := client.CloneVM(ctx, cfg.Node, cfg.VMID, cfg.NewName, cfg.NewID); err != nil {
		return nil, fmt.Errorf("clone vm %d to %s (new id %d) on node %s: %w", cfg.VMID, cfg.NewName, cfg.NewID, cfg.Node, err)
	}

	output := map[string]any{
		"status":      "cloned",
		"clusterId":   cluster.ID,
		"clusterName": cluster.Name,
		"node":        cfg.Node,
		"vmid":        cfg.VMID,
		"newName":     cfg.NewName,
		"newId":       cfg.NewID,
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *VMCloneAction) Validate(config json.RawMessage) error {
	var cfg vmCloneConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.Node == "" {
		return fmt.Errorf("node is required")
	}
	if cfg.VMID == 0 {
		return fmt.Errorf("vmid is required")
	}
	if cfg.NewName == "" {
		return fmt.Errorf("newName is required")
	}
	if cfg.NewID == 0 {
		return fmt.Errorf("newId is required")
	}
	if cfg.ClusterID == "" {
		return fmt.Errorf("clusterId is required")
	}
	return nil
}

type HTTPAction struct{}

type httpActionConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (e *HTTPAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	var cfg httpActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	if cfg.Method == "" {
		cfg.Method = "GET"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	var bodyReader io.Reader
	if cfg.Body != "" {
		bodyReader = bytes.NewReader([]byte(cfg.Body))
	}

	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	output := map[string]any{
		"status_code": resp.StatusCode,
		"url":         cfg.URL,
		"method":      cfg.Method,
		"response":    parseHTTPResponseBody(respBody, resp.Header.Get("Content-Type")),
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *HTTPAction) Validate(config json.RawMessage) error {
	var cfg httpActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.URL == "" {
		return fmt.Errorf("url is required")
	}
	return nil
}

func parseHTTPResponseBody(respBody []byte, contentType string) any {
	trimmed := bytes.TrimSpace(respBody)
	if len(trimmed) == 0 {
		return nil
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err == nil {
		return decoded
	}

	mediaType := normalizeHTTPContentType(contentType)
	if isHTMLContentType(mediaType) {
		return string(respBody)
	}

	if isXMLContentType(mediaType) || looksLikeXML(trimmed) {
		if decodedXML, err := parseXMLResponseBody(trimmed); err == nil {
			return decodedXML
		}
	}

	return string(respBody)
}

func normalizeHTTPContentType(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return ""
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		return strings.ToLower(strings.TrimSpace(mediaType))
	}

	return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
}

func isHTMLContentType(contentType string) bool {
	return contentType == "text/html"
}

func isXMLContentType(contentType string) bool {
	return contentType == "application/xml" || contentType == "text/xml" || strings.HasSuffix(contentType, "+xml")
}

func looksLikeXML(respBody []byte) bool {
	lower := bytes.ToLower(bytes.TrimSpace(respBody))
	if !bytes.HasPrefix(lower, []byte("<")) {
		return false
	}
	if bytes.HasPrefix(lower, []byte("<!doctype html")) || bytes.HasPrefix(lower, []byte("<html")) {
		return false
	}
	return true
}

type xmlResponseElement struct {
	name     string
	attrs    map[string]string
	children map[string][]any
	text     []string
}

func (e *xmlResponseElement) addChild(name string, value any) {
	if e.children == nil {
		e.children = make(map[string][]any)
	}
	e.children[name] = append(e.children[name], value)
}

func (e *xmlResponseElement) addText(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return
	}
	e.text = append(e.text, trimmed)
}

func (e *xmlResponseElement) value() any {
	text := strings.Join(e.text, " ")
	if len(e.attrs) == 0 && len(e.children) == 0 {
		if text == "" {
			return ""
		}
		return text
	}

	result := make(map[string]any, len(e.attrs)+len(e.children)+1)
	for key, value := range e.attrs {
		result["@"+key] = value
	}
	for key, values := range e.children {
		if len(values) == 1 {
			result[key] = values[0]
			continue
		}
		result[key] = values
	}
	if text != "" {
		result["#text"] = text
	}

	return result
}

func parseXMLResponseBody(respBody []byte) (map[string]any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(respBody))
	stack := make([]*xmlResponseElement, 0, 8)
	var root *xmlResponseElement

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch current := token.(type) {
		case xml.StartElement:
			element := &xmlResponseElement{name: current.Name.Local}
			if len(current.Attr) > 0 {
				element.attrs = make(map[string]string, len(current.Attr))
				for _, attr := range current.Attr {
					element.attrs[attr.Name.Local] = attr.Value
				}
			}
			stack = append(stack, element)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("xml response has unexpected closing tag %q", current.Name.Local)
			}

			element := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if len(stack) == 0 {
				root = element
				continue
			}

			parent := stack[len(stack)-1]
			parent.addChild(element.name, element.value())
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			stack[len(stack)-1].addText(string(current))
		}
	}

	if root == nil {
		return nil, fmt.Errorf("xml response does not contain a root element")
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("xml response ended before all elements were closed")
	}

	return map[string]any{root.name: root.value()}, nil
}

type ShellCommandAction struct {
	Runner shellcmd.Runner
}

func (e *ShellCommandAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	return (&ShellCommandToolNode{Runner: e.Runner}).Execute(ctx, config, input)
}

func (e *ShellCommandAction) Validate(config json.RawMessage) error {
	return (&ShellCommandToolNode{Runner: e.Runner}).Validate(config)
}

type ChannelSendAction struct {
	Channels ChannelStore
	Contacts ChannelContactStore
	Sender   ChannelMessageSender
}

type channelSendConfig struct {
	ChannelID string `json:"channelId"`
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
}

func (e *ChannelSendAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Channels == nil || e.Sender == nil {
		return nil, fmt.Errorf("channel sender is not configured")
	}

	var cfg channelSendConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	cfg.ChannelID = resolveChannelID(cfg.ChannelID, input)
	if strings.TrimSpace(cfg.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	channel, contact, recipientID, chatID, err := resolveChannelTarget(ctx, e.Channels, e.Contacts, cfg.ChannelID, cfg.Recipient, input)
	if err != nil {
		return nil, err
	}

	result, err := e.Sender.SendMessage(ctx, channel, chatID, cfg.Message)
	if err != nil {
		return nil, fmt.Errorf("send message to channel %s: %w", channel.Name, err)
	}

	output := map[string]any{
		"status":      "sent",
		"channelId":   channel.ID,
		"channelName": channel.Name,
		"recipient":   recipientID,
		"chat_id":     chatID,
		"message":     cfg.Message,
		"response":    result,
	}
	if contact != nil {
		output["contact_id"] = contact.ID
	}
	if messageID := extractMessageID(result); messageID != "" {
		output["messageId"] = messageID
		output["message_id"] = messageID
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ChannelSendAction) Validate(config json.RawMessage) error {
	var cfg channelSendConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.ChannelID == "" {
		return fmt.Errorf("channelId is required")
	}
	if strings.TrimSpace(cfg.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

func resolveChannelRecipient(input map[string]any) string {
	if value, ok := input["contact_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := input["chat_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value, ok := input["external_chat_id"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return ""
}

type ChannelEditAction struct {
	Channels ChannelStore
	Contacts ChannelContactStore
	Sender   ChannelMessageSender
}

type channelReplyConfig struct {
	ChannelID        string `json:"channelId"`
	Recipient        string `json:"recipient"`
	ReplyToMessageID string `json:"replyToMessageId"`
	Message          string `json:"message"`
}

type ChannelReplyAction struct {
	Channels ChannelStore
	Contacts ChannelContactStore
	Sender   ChannelMessageSender
}

func (e *ChannelReplyAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Channels == nil || e.Sender == nil {
		return nil, fmt.Errorf("channel replier is not configured")
	}

	var cfg channelReplyConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	cfg.ChannelID = resolveChannelID(cfg.ChannelID, input)
	if strings.TrimSpace(cfg.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	channel, contact, recipientID, chatID, err := resolveChannelTarget(ctx, e.Channels, e.Contacts, cfg.ChannelID, cfg.Recipient, input)
	if err != nil {
		return nil, err
	}

	replyToMessageID := resolveChannelMessageID(cfg.ReplyToMessageID, input)
	if replyToMessageID == "" {
		return nil, fmt.Errorf("replyToMessageId is required")
	}

	result, err := e.Sender.ReplyMessage(ctx, channel, chatID, replyToMessageID, cfg.Message)
	if err != nil {
		return nil, fmt.Errorf("reply to message in channel %s: %w", channel.Name, err)
	}

	output := map[string]any{
		"status":              "replied",
		"channelId":           channel.ID,
		"channelName":         channel.Name,
		"recipient":           recipientID,
		"chat_id":             chatID,
		"replyToMessageId":    replyToMessageID,
		"reply_to_message_id": replyToMessageID,
		"message":             cfg.Message,
		"response":            result,
	}
	if contact != nil {
		output["contact_id"] = contact.ID
	}
	if messageID := extractMessageID(result); messageID != "" {
		output["messageId"] = messageID
		output["message_id"] = messageID
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ChannelReplyAction) Validate(config json.RawMessage) error {
	var cfg channelReplyConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return fmt.Errorf("channelId is required")
	}
	if strings.TrimSpace(cfg.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

type channelEditConfig struct {
	ChannelID string `json:"channelId"`
	Recipient string `json:"recipient"`
	MessageID string `json:"messageId"`
	Message   string `json:"message"`
}

func (e *ChannelEditAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Channels == nil || e.Sender == nil {
		return nil, fmt.Errorf("channel editor is not configured")
	}

	var cfg channelEditConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return nil, fmt.Errorf("render config: %w", err)
	}

	cfg.ChannelID = resolveChannelID(cfg.ChannelID, input)
	if strings.TrimSpace(cfg.Message) == "" {
		return nil, fmt.Errorf("message is required")
	}

	channel, contact, recipientID, chatID, err := resolveChannelTarget(ctx, e.Channels, e.Contacts, cfg.ChannelID, cfg.Recipient, input)
	if err != nil {
		return nil, err
	}

	messageID := resolveChannelMessageID(cfg.MessageID, input)
	if messageID == "" {
		return nil, fmt.Errorf("messageId is required")
	}

	result, err := e.Sender.EditMessage(ctx, channel, chatID, messageID, cfg.Message)
	if err != nil {
		return nil, fmt.Errorf("edit message in channel %s: %w", channel.Name, err)
	}

	output := map[string]any{
		"status":      "edited",
		"channelId":   channel.ID,
		"channelName": channel.Name,
		"recipient":   recipientID,
		"chat_id":     chatID,
		"messageId":   messageID,
		"message_id":  messageID,
		"message":     cfg.Message,
		"response":    result,
	}
	if contact != nil {
		output["contact_id"] = contact.ID
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ChannelEditAction) Validate(config json.RawMessage) error {
	var cfg channelEditConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return fmt.Errorf("channelId is required")
	}
	if strings.TrimSpace(cfg.Message) == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

type ChannelSendAndWaitAction struct {
	Channels ChannelStore
	Contacts ChannelContactStore
	Sender   ChannelMessageSender
	Waiter   ChannelReplyWaiter
}

type channelSendAndWaitConfig struct {
	ChannelID      string `json:"channelId"`
	Recipient      string `json:"recipient"`
	Message        string `json:"message"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func (e *ChannelSendAndWaitAction) Execute(ctx context.Context, config json.RawMessage, input map[string]any) (*node.NodeResult, error) {
	if e.Channels == nil || e.Sender == nil || e.Waiter == nil {
		return nil, fmt.Errorf("channel wait sender is not configured")
	}

	cfg, err := parseChannelSendAndWaitConfig(ctx, config, input)
	if err != nil {
		return nil, err
	}

	channel, contact, recipientID, chatID, err := resolveChannelTarget(ctx, e.Channels, e.Contacts, cfg.ChannelID, cfg.Recipient, input)
	if err != nil {
		return nil, err
	}

	sendResult, err := e.Sender.SendMessage(ctx, channel, chatID, cfg.Message)
	if err != nil {
		return nil, fmt.Errorf("send message to channel %s: %w", channel.Name, err)
	}

	waitCtx := ctx
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	contactID := ""
	if contact != nil {
		contactID = contact.ID
	}

	reply, err := e.Waiter.WaitForReply(waitCtx, channel.ID, contactID, chatID, extractMessageID(sendResult), timeout)
	if err != nil {
		return nil, fmt.Errorf("wait for reply on channel %s: %w", channel.Name, err)
	}

	output := map[string]any{
		"status":       "received",
		"channelId":    channel.ID,
		"channelName":  channel.Name,
		"recipient":    recipientID,
		"chat_id":      chatID,
		"message":      cfg.Message,
		"sent_message": sendResult,
		"reply":        reply,
	}
	if contact != nil {
		output["contact_id"] = contact.ID
	}
	if messageID := extractMessageID(sendResult); messageID != "" {
		output["sentMessageId"] = messageID
		output["sent_message_id"] = messageID
	}
	data, _ := json.Marshal(output)
	return &node.NodeResult{Output: data}, nil
}

func (e *ChannelSendAndWaitAction) Validate(config json.RawMessage) error {
	var cfg channelSendAndWaitConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if strings.TrimSpace(cfg.ChannelID) == "" {
		return fmt.Errorf("channelId is required")
	}
	if strings.TrimSpace(cfg.Message) == "" {
		return fmt.Errorf("message is required")
	}
	if cfg.TimeoutSeconds < 0 {
		return fmt.Errorf("timeoutSeconds must be 0 or greater")
	}
	return nil
}

func parseChannelSendAndWaitConfig(ctx context.Context, config json.RawMessage, input map[string]any) (channelSendAndWaitConfig, error) {
	var cfg channelSendAndWaitConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return channelSendAndWaitConfig{}, fmt.Errorf("parse config: %w", err)
	}
	if err := templating.RenderStringsWithContext(ctx, &cfg, input); err != nil {
		return channelSendAndWaitConfig{}, fmt.Errorf("render config: %w", err)
	}

	cfg.ChannelID = resolveChannelID(cfg.ChannelID, input)
	if strings.TrimSpace(cfg.Message) == "" {
		return channelSendAndWaitConfig{}, fmt.Errorf("message is required")
	}
	if cfg.TimeoutSeconds == 0 {
		cfg.TimeoutSeconds = 300
	}
	if cfg.TimeoutSeconds < 0 {
		return channelSendAndWaitConfig{}, fmt.Errorf("timeoutSeconds must be 0 or greater")
	}

	return cfg, nil
}

func resolveChannelTarget(
	ctx context.Context,
	channelStore ChannelStore,
	contactStore ChannelContactStore,
	channelID string,
	recipient string,
	input map[string]any,
) (*models.Channel, *models.ChannelContact, string, string, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, nil, "", "", fmt.Errorf("channelId is required")
	}

	channel, err := channelStore.GetByID(ctx, channelID)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if channel == nil {
		return nil, nil, "", "", fmt.Errorf("channel %s not found", channelID)
	}

	recipientID := strings.TrimSpace(recipient)
	if recipientID == "" {
		recipientID = resolveChannelRecipient(input)
	}
	if recipientID == "" {
		return nil, nil, "", "", fmt.Errorf("recipient is required")
	}

	chatID := recipientID
	var contact *models.ChannelContact
	if contactStore != nil {
		if loadedContact, err := contactStore.GetByID(ctx, recipientID); err == nil && loadedContact != nil {
			if loadedContact.ChannelID != channel.ID {
				return nil, nil, "", "", fmt.Errorf("contact does not belong to channel %s", channel.Name)
			}
			contact = loadedContact
			chatID = loadedContact.ExternalChatID
		}
	}

	return channel, contact, recipientID, chatID, nil
}

func resolveChannelMessageID(configMessageID string, input map[string]any) string {
	if value := strings.TrimSpace(configMessageID); value != "" {
		return value
	}

	for _, key := range []string{"message_id", "messageId", "sent_message_id", "sentMessageId"} {
		if value := stringifyChannelValue(input[key]); value != "" {
			return value
		}
	}

	for _, key := range []string{"response", "sent_message", "sentMessage", "message"} {
		if value := extractMessageID(asMap(input[key])); value != "" {
			return value
		}
	}

	return ""
}

func extractMessageID(payload map[string]any) string {
	if len(payload) == 0 {
		return ""
	}

	for _, key := range []string{"message_id", "id"} {
		if value := stringifyChannelValue(payload[key]); value != "" {
			return value
		}
	}

	return ""
}

func asMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func stringifyChannelValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed != 0 {
			return fmt.Sprintf("%.0f", typed)
		}
	case int:
		if typed != 0 {
			return fmt.Sprintf("%d", typed)
		}
	case int64:
		if typed != 0 {
			return fmt.Sprintf("%d", typed)
		}
	case json.Number:
		return strings.TrimSpace(typed.String())
	}

	return ""
}
