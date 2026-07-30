# Tool call format (SGLang / Qwen GGUF)

This workspace often runs VT Code against **SGLang**
(`http://127.0.0.1:30000/v1`). Dense Qwen GGUF builds frequently **do not** emit
native OpenAI `tool_calls`. VT Code recovers **text tools** only for specific
dialects. If you use the wrong dialect, the raw markup is shown as the final
reply and **nothing runs**.

## Required text-tool dialect (use this)

Prefer the `default_api.` prefix (most reliable with VT Code recovery):

```text
default_api.exec_command(cmd="ls -la")
default_api.exec_command(cmd="cat README.md", workdir=".")
```

Tagged form is also OK **only if the tool name is outside JSON**:

```text
<tool_call>exec_command
{"cmd": "ls -la", "workdir": "."}
</tool_call>
```

## Forbidden (breaks replies)

These formats are **not** recovered. They appear as the final answer:

```text
<!-- BAD: name inside JSON -->
<tool_call>
{"name": "exec_command", "arguments": {"cmd": "ls"}}
</tool_call>
```

```text
<!-- BAD: OpenAI-ish array with parameters -->
[{"name": "list_dir", "parameters": {"path": "."}}]
```

## Parameter names

For `exec_command` use schema fields: **`cmd`** (required), optional `workdir`,
`tty`. Do not use `command`, `parameters`, or an `arguments` wrapper object.

Available progressive tools typically include: `exec_command`, `write_stdin`,
`apply_patch`. Prefer `exec_command` with `ls`/`cat`/`rg` over inventing
`list_dir` / `read_file` if those tools are not on the wire.

## After tools run

Give a normal answer. Never leave `<tool_call>` or `default_api.` lines in the
final response.
