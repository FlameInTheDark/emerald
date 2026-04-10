package channels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/FlameInTheDark/emerald/internal/db/models"
	"github.com/FlameInTheDark/emerald/internal/db/query"
	"github.com/FlameInTheDark/emerald/internal/node/trigger"
)

type MessageDispatch func(ctx context.Context, event trigger.ChannelEvent) error

type channelRuntime interface {
	Run(ctx context.Context) error
	SendMessage(ctx context.Context, chatID string, text string, buttonURL string) (map[string]any, error)
	ReplyMessage(ctx context.Context, chatID string, replyToMessageID string, text string) (map[string]any, error)
	EditMessage(ctx context.Context, chatID string, messageID string, text string) (map[string]any, error)
	Close() error
}

type activeWorker struct {
	cancel  context.CancelFunc
	runtime channelRuntime
}

type pendingReplyWaiter struct {
	id            string
	channelID     string
	contactID     string
	chatID        string
	sentMessageID string
	responseCh    chan map[string]any
}

type Service struct {
	channelStore *query.ChannelStore
	contactStore *query.ChannelContactStore
	dispatch     MessageDispatch
	httpClient   *http.Client

	mu         sync.Mutex
	rootCtx    context.Context
	rootCancel context.CancelFunc
	workers    map[string]*activeWorker

	waitMu  sync.Mutex
	waiters map[string][]*pendingReplyWaiter
}

func NewService(
	channelStore *query.ChannelStore,
	contactStore *query.ChannelContactStore,
	dispatch MessageDispatch,
) *Service {
	return &Service{
		channelStore: channelStore,
		contactStore: contactStore,
		dispatch:     dispatch,
		httpClient: &http.Client{
			Timeout: 40 * time.Second,
		},
		workers: make(map[string]*activeWorker),
		waiters: make(map[string][]*pendingReplyWaiter),
	}
}

func (s *Service) Start() error {
	s.mu.Lock()
	if s.rootCancel == nil {
		s.rootCtx, s.rootCancel = context.WithCancel(context.Background())
	}
	s.mu.Unlock()

	return s.Reload(context.Background())
}

func (s *Service) Stop() {
	s.mu.Lock()
	workers := s.workers
	rootCancel := s.rootCancel

	s.workers = make(map[string]*activeWorker)
	s.rootCtx = nil
	s.rootCancel = nil
	s.mu.Unlock()

	if rootCancel != nil {
		rootCancel()
	}
	s.stopWorkers(workers)
}

func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	if s.rootCancel == nil {
		s.rootCtx, s.rootCancel = context.WithCancel(context.Background())
	}

	rootCtx := s.rootCtx
	oldWorkers := s.workers
	s.workers = make(map[string]*activeWorker)
	s.mu.Unlock()

	s.stopWorkers(oldWorkers)

	channelList, err := s.channelStore.ListEnabled(ctx)
	if err != nil {
		return err
	}

	for _, channel := range channelList {
		workerCtx, cancel := context.WithCancel(rootCtx)
		worker := &activeWorker{cancel: cancel}

		s.mu.Lock()
		s.workers[channel.ID] = worker
		s.mu.Unlock()

		go s.runChannelWorker(workerCtx, channel, worker)
	}

	return nil
}

func (s *Service) SendMessage(ctx context.Context, channel *models.Channel, chatID string, text string) (map[string]any, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is required")
	}
	if !channel.Enabled {
		return nil, fmt.Errorf("channel %s is not active", channel.Name)
	}

	return s.sendMessage(ctx, channel, chatID, text, "")
}

func (s *Service) ReplyMessage(ctx context.Context, channel *models.Channel, chatID string, replyToMessageID string, text string) (map[string]any, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is required")
	}
	if !channel.Enabled {
		return nil, fmt.Errorf("channel %s is not active", channel.Name)
	}

	return s.replyMessage(ctx, channel, chatID, replyToMessageID, text)
}

func (s *Service) EditMessage(ctx context.Context, channel *models.Channel, chatID string, messageID string, text string) (map[string]any, error) {
	if channel == nil {
		return nil, fmt.Errorf("channel is required")
	}
	if !channel.Enabled {
		return nil, fmt.Errorf("channel %s is not active", channel.Name)
	}

	return s.editMessage(ctx, channel, chatID, messageID, text)
}

func (s *Service) ConnectContact(ctx context.Context, channelID string, code string) (*models.ChannelContact, error) {
	contact, err := s.contactStore.GetByConnectionCode(ctx, channelID, strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	if contact == nil {
		return nil, fmt.Errorf("connection code not found")
	}
	if contact.ConnectedAt != nil {
		return contact, nil
	}
	if contact.CodeExpiresAt != nil && contact.CodeExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("connection code expired")
	}

	now := time.Now()
	if err := s.contactStore.Connect(ctx, contact.ID, now); err != nil {
		return nil, err
	}

	return s.contactStore.GetByID(ctx, contact.ID)
}

func (s *Service) WaitForReply(
	ctx context.Context,
	channelID string,
	contactID string,
	chatID string,
	sentMessageID string,
	timeout time.Duration,
) (map[string]any, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, fmt.Errorf("channelId is required")
	}
	if strings.TrimSpace(chatID) == "" {
		return nil, fmt.Errorf("chat_id is required")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	waiter := &pendingReplyWaiter{
		id:            uuid.New().String(),
		channelID:     strings.TrimSpace(channelID),
		contactID:     strings.TrimSpace(contactID),
		chatID:        strings.TrimSpace(chatID),
		sentMessageID: strings.TrimSpace(sentMessageID),
		responseCh:    make(chan map[string]any, 1),
	}

	s.registerWaiter(waiter)
	defer s.unregisterWaiter(waiter)

	select {
	case response := <-waiter.responseCh:
		return response, nil
	case <-ctx.Done():
		if timeout > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out waiting for reply after %s", timeout.Round(time.Second))
		}
		return nil, fmt.Errorf("wait for reply cancelled: %w", ctx.Err())
	}
}

func (s *Service) sendMessage(ctx context.Context, channel *models.Channel, chatID string, text string, buttonURL string) (map[string]any, error) {
	runtime, cleanup, err := s.runtimeForChannel(*channel)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return runtime.SendMessage(ctx, chatID, text, buttonURL)
}

func (s *Service) editMessage(ctx context.Context, channel *models.Channel, chatID string, messageID string, text string) (map[string]any, error) {
	runtime, cleanup, err := s.runtimeForChannel(*channel)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return runtime.EditMessage(ctx, chatID, messageID, text)
}

func (s *Service) replyMessage(ctx context.Context, channel *models.Channel, chatID string, replyToMessageID string, text string) (map[string]any, error) {
	runtime, cleanup, err := s.runtimeForChannel(*channel)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return runtime.ReplyMessage(ctx, chatID, replyToMessageID, text)
}

func (s *Service) runtimeForChannel(channel models.Channel) (channelRuntime, func(), error) {
	s.mu.Lock()
	worker := s.workers[channel.ID]
	var runtime channelRuntime
	if worker != nil {
		runtime = worker.runtime
	}
	s.mu.Unlock()

	if runtime != nil {
		return runtime, func() {}, nil
	}

	runtime, err := s.newRuntime(channel)
	if err != nil {
		return nil, nil, err
	}

	return runtime, func() {
		if err := runtime.Close(); err != nil {
			log.Printf("failed to close transient %s channel runtime %s: %v", channel.Type, channel.Name, err)
		}
	}, nil
}

func (s *Service) newRuntime(channel models.Channel) (channelRuntime, error) {
	cfg, err := ParseConfig(&channel)
	if err != nil {
		return nil, err
	}

	switch channel.Type {
	case TypeTelegram:
		return newTelegramRuntime(s, channel, cfg)
	case TypeDiscord:
		return newDiscordRuntime(s, channel, cfg)
	default:
		return nil, fmt.Errorf("unsupported channel type: %s", channel.Type)
	}
}

func (s *Service) runChannelWorker(ctx context.Context, channel models.Channel, worker *activeWorker) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		runtime, err := s.newRuntime(channel)
		if err != nil {
			log.Printf("channel worker %s (%s) failed to initialize: %v", channel.Name, channel.Type, err)
			if !sleepContext(ctx, 5*time.Second) {
				return
			}
			continue
		}

		if !s.attachRuntime(channel.ID, worker, runtime) {
			_ = runtime.Close()
			return
		}

		err = runtime.Run(ctx)
		s.detachRuntime(channel.ID, worker, runtime)

		if closeErr := runtime.Close(); closeErr != nil && ctx.Err() == nil {
			log.Printf("channel worker %s (%s) failed to close: %v", channel.Name, channel.Type, closeErr)
		}

		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("channel worker %s (%s) failed: %v", channel.Name, channel.Type, err)
		}

		if !sleepContext(ctx, 5*time.Second) {
			return
		}
	}
}

func (s *Service) attachRuntime(channelID string, worker *activeWorker, runtime channelRuntime) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.workers[channelID]
	if current != worker {
		return false
	}

	worker.runtime = runtime
	return true
}

func (s *Service) detachRuntime(channelID string, worker *activeWorker, runtime channelRuntime) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.workers[channelID]
	if current != worker || worker.runtime != runtime {
		return
	}

	worker.runtime = nil
}

func (s *Service) stopWorkers(workers map[string]*activeWorker) {
	for _, worker := range workers {
		if worker == nil {
			continue
		}
		worker.cancel()
	}

	for _, worker := range workers {
		if worker == nil || worker.runtime == nil {
			continue
		}
		if err := worker.runtime.Close(); err != nil {
			log.Printf("failed to close channel runtime: %v", err)
		}
	}
}

func (s *Service) handleIncomingMessage(ctx context.Context, channel *models.Channel, message IncomingMessage) error {
	now := time.Now()

	contact, err := s.contactStore.GetByChannelAndExternalUser(ctx, channel.ID, message.ExternalUserID)
	if err != nil {
		return err
	}

	username := optionalString(message.Username)
	displayName := optionalString(message.DisplayName)

	if contact == nil {
		code, expiresAt := generateConnectionCode()
		contact = &models.ChannelContact{
			ChannelID:      channel.ID,
			ExternalUserID: message.ExternalUserID,
			ExternalChatID: message.ExternalChatID,
			Username:       username,
			DisplayName:    displayName,
			ConnectionCode: &code,
			CodeExpiresAt:  &expiresAt,
			LastMessageAt:  &now,
		}

		if err := s.contactStore.Create(ctx, contact); err != nil {
			return err
		}
	} else {
		contact.ExternalChatID = message.ExternalChatID
		contact.Username = username
		contact.DisplayName = displayName
		contact.LastMessageAt = &now

		if contact.ConnectedAt == nil && (contact.ConnectionCode == nil || contact.CodeExpiresAt == nil || contact.CodeExpiresAt.Before(now)) {
			code, expiresAt := generateConnectionCode()
			contact.ConnectionCode = &code
			contact.CodeExpiresAt = &expiresAt
		}

		if err := s.contactStore.Update(ctx, contact); err != nil {
			return err
		}
	}

	if contact.ConnectedAt == nil {
		return s.sendWelcome(ctx, channel, contact, message.ExternalChatID)
	}

	if s.tryDeliverWaiter(channel, contact, message) {
		return nil
	}

	if s.dispatch == nil {
		return nil
	}

	return s.dispatch(ctx, trigger.ChannelEvent{
		ChannelID:      channel.ID,
		ChannelName:    channel.Name,
		ChannelType:    channel.Type,
		ContactID:      contact.ID,
		ExternalUserID: message.ExternalUserID,
		ExternalChatID: message.ExternalChatID,
		Username:       message.Username,
		DisplayName:    message.DisplayName,
		Text:           message.Text,
		Message:        message.Raw,
	})
}

func (s *Service) sendWelcome(ctx context.Context, channel *models.Channel, contact *models.ChannelContact, chatID string) error {
	code := ""
	if contact.ConnectionCode != nil {
		code = *contact.ConnectionCode
	}

	text := strings.TrimSpace(channel.WelcomeMessage)
	if text == "" {
		text = "Welcome! Use this one-time code to connect this chat to Emerald."
	}
	text = fmt.Sprintf("%s\n\nCode: %s", text, code)

	buttonURL := ""
	if channel.ConnectURL != nil {
		switch channel.Type {
		case TypeTelegram:
			buttonURL = buildTelegramConnectURL(*channel.ConnectURL, channel.ID)
		case TypeDiscord:
			buttonURL = appendChannelIDParam(*channel.ConnectURL, channel.ID)
		}
	}

	_, err := s.sendMessage(ctx, channel, chatID, text, buttonURL)
	return err
}

func generateConnectionCode() (string, time.Time) {
	code := strings.ToUpper(strings.ReplaceAll(uuid.New().String()[:6], "-", ""))
	return code, time.Now().Add(15 * time.Minute)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) registerWaiter(waiter *pendingReplyWaiter) {
	if waiter == nil {
		return
	}

	key := waiterKey(waiter.channelID, waiter.chatID)

	s.waitMu.Lock()
	defer s.waitMu.Unlock()

	s.waiters[key] = append(s.waiters[key], waiter)
}

func (s *Service) unregisterWaiter(waiter *pendingReplyWaiter) {
	if waiter == nil {
		return
	}

	key := waiterKey(waiter.channelID, waiter.chatID)

	s.waitMu.Lock()
	defer s.waitMu.Unlock()

	waiters := s.waiters[key]
	for index, current := range waiters {
		if current == nil || current.id != waiter.id {
			continue
		}

		waiters = append(waiters[:index], waiters[index+1:]...)
		if len(waiters) == 0 {
			delete(s.waiters, key)
		} else {
			s.waiters[key] = waiters
		}
		return
	}
}

func (s *Service) tryDeliverWaiter(channel *models.Channel, contact *models.ChannelContact, message IncomingMessage) bool {
	if channel == nil {
		return false
	}

	key := waiterKey(channel.ID, message.ExternalChatID)

	s.waitMu.Lock()
	waiters := s.waiters[key]
	if len(waiters) == 0 {
		s.waitMu.Unlock()
		return false
	}

	selectedIndex := s.selectWaiter(waiters, contact, message)
	if selectedIndex < 0 {
		s.waitMu.Unlock()
		return false
	}

	waiter := waiters[selectedIndex]
	waiters = append(waiters[:selectedIndex], waiters[selectedIndex+1:]...)
	if len(waiters) == 0 {
		delete(s.waiters, key)
	} else {
		s.waiters[key] = waiters
	}
	s.waitMu.Unlock()

	select {
	case waiter.responseCh <- buildReplyPayload(channel, contact, message):
	default:
	}

	return true
}

func (s *Service) selectWaiter(waiters []*pendingReplyWaiter, contact *models.ChannelContact, message IncomingMessage) int {
	replyToMessageID := strings.TrimSpace(message.ReplyToMessage)

	if replyToMessageID != "" {
		for index, waiter := range waiters {
			if waiter == nil || !waiter.matches(contact, message, true) {
				continue
			}
			return index
		}
	}

	for index, waiter := range waiters {
		if waiter == nil || !waiter.matches(contact, message, false) {
			continue
		}
		return index
	}

	return -1
}

func (w *pendingReplyWaiter) matches(contact *models.ChannelContact, message IncomingMessage, requireReplyReference bool) bool {
	if w == nil {
		return false
	}
	if w.contactID != "" {
		if contact == nil || contact.ID != w.contactID {
			return false
		}
	}
	if w.chatID != "" && w.chatID != strings.TrimSpace(message.ExternalChatID) {
		return false
	}
	if requireReplyReference {
		return w.sentMessageID != "" && w.sentMessageID == strings.TrimSpace(message.ReplyToMessage)
	}
	return true
}

func waiterKey(channelID string, chatID string) string {
	return strings.TrimSpace(channelID) + ":" + strings.TrimSpace(chatID)
}

func buildReplyPayload(channel *models.Channel, contact *models.ChannelContact, message IncomingMessage) map[string]any {
	payload := map[string]any{
		"channel_id":          channel.ID,
		"channel_name":        channel.Name,
		"channel_type":        channel.Type,
		"external_user_id":    message.ExternalUserID,
		"external_chat_id":    message.ExternalChatID,
		"chat_id":             message.ExternalChatID,
		"user_id":             message.ExternalUserID,
		"text":                message.Text,
		"message_id":          message.MessageID,
		"reply_to_message_id": message.ReplyToMessage,
		"message":             cloneMessagePayload(message.Raw),
	}
	if strings.TrimSpace(message.Username) != "" {
		payload["username"] = message.Username
	}
	if strings.TrimSpace(message.DisplayName) != "" {
		payload["display_name"] = message.DisplayName
	}
	if contact != nil {
		payload["contact_id"] = contact.ID
	}

	return payload
}

func cloneMessagePayload(value map[string]any) map[string]any {
	if len(value) == 0 {
		return make(map[string]any)
	}

	data, err := json.Marshal(value)
	if err != nil {
		cloned := make(map[string]any, len(value))
		for key, raw := range value {
			cloned[key] = raw
		}
		return cloned
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		cloned := make(map[string]any, len(value))
		for key, raw := range value {
			cloned[key] = raw
		}
		return cloned
	}

	return decoded
}
