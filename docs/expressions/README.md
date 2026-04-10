# Expressions

Emerald uses [Expr](https://expr-lang.org/) for boolean logic in:

- `logic:condition`
- condition-based `logic:switch`

## Official Expr Documentation

- [Getting Started](https://expr-lang.org/docs/getting-started)
- [Language Definition](https://expr-lang.org/docs/language-definition)
- [Playground](https://expr-lang.org/playground)

## What Emerald Passes Into Expressions

When Emerald evaluates an expression:

- `input` contains the full current payload.
- Top-level payload keys are also exposed directly.

That means these two styles are both valid when the relevant keys exist:

```text
input.status_code == 200
status_code == 200
```

Using `input.<field>` is usually clearer when you want the data source to stay obvious.

## Common Examples

```text
input.status == "ready"
retries > 3
input.response.status_code >= 200 && input.response.status_code < 300
input.cluster == "prod" && input.enabled == true
len(input.items) > 0
```

## How Expressions Differ From Templates

Expressions and templates solve different problems:

- Use Expr syntax in logic-node expression fields.
- Use `{{ ... }}` templates in string-based config fields such as prompts, messages, URLs, headers, and request bodies.

Correct:

```text
input.status_code == 200
```

Incorrect:

```text
{{input.status_code == 200}}
```

## Good Practices

- Keep conditions short and specific.
- Compare explicit fields instead of serializing whole objects.
- Prefer `input.<field>` when there are several similarly named keys in the payload.
- Split complicated branching into multiple logic nodes instead of one very dense expression.

## Common Mistakes

- Returning a string or number when a boolean is required.
- Mixing template braces into a normal expression.
- Using Lua or JavaScript syntax inside Expr expressions.
- Forgetting that upstream logic nodes may wrap earlier payloads under their own `input` key.

Example:

- After an HTTP node, `input.response` may exist directly.
- After a `logic:switch`, the switch output contains the previous payload under `input`, so later templates or expressions may need `input.input.response`.
