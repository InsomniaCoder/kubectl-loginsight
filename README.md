# kubectl-loginsight

A kubectl plugin that analyzes Kubernetes logs using a local LLM. Pipe logs through it to get an instant summary or answer a specific question — no more `grep` needle-in-a-haystack.

## Installation

### Prerequisites

- [LM Studio](https://lmstudio.ai) with a model loaded and the local server running, or any other OpenAI-compatible LLM server (Ollama, vLLM, real OpenAI, etc.)

### Recommended machine specs (Mac)

**Baseline / reference setup:** MacBook Pro M2 Pro, 32 GB unified memory, running `qwen/qwen3-9b` GGUF via LM Studio.

### Install

```bash
# Install directly
go install github.com/InsomniaCoder/kubectl-loginsight@latest
```

Make sure `~/go/bin` is in your PATH:
```bash
export PATH="$PATH:$HOME/go/bin"  # add to ~/.zshrc to make it permanent
```

Verify kubectl discovers it:
```bash
kubectl plugin list  # should show /Users/<you>/go/bin/kubectl-loginsight
```

> **How kubectl discovers plugins:** kubectl scans every directory in `$PATH` for executables named `kubectl-*` and exposes them as subcommands. Because this binary is named `kubectl-loginsight`, you can invoke it as either `kubectl loginsight` or `kubectl-loginsight`.

## Usage

```bash
# Summarize what's happening in the logs
kubectl logs <pod> | kubectl-loginsight --model qwen/qwen3.5-9b --base-url http://localhost:1234/v1

# Ask a specific question
kubectl logs <pod> | kubectl-loginsight --model qwen/qwen3.5-9b --base-url http://localhost:1234/v1 -q "why did the pod crash?"

# Read from a saved log file
kubectl logs <pod> > ./app.log
kubectl-loginsight --file ./app.log --model qwen/qwen3.5-9b --base-url http://localhost:1234/v1 -q "any OOMKilled signs?"
```

## Flags

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--model` / `-m` | Yes | — | Model name to pass to the LLM API |
| `--question` / `-q` | No | — | Question to ask. If omitted, summarize mode is used |
| `--base-url` | Yes | `http://localhost:1234/v1` | OpenAI-compatible API base URL |
| `--api-key` | No | `test` | API key (not needed for local models) |
| `--max-tokens` | No | `8192` | Max tokens of log content to send  |
| `--file` / `-f` | No | — | Read logs from file instead of stdin |

## Config File

To avoid typing flags every time, create `~/.kube/log-insight.yaml`:

```yaml
model: qwen/qwen3.5-9b
base-url: http://localhost:1234/v1
api-key: test
max-tokens: 8192
```

Flags always override config file values.

## Using with other LLM backends

Since `--base-url` accepts any OpenAI-compatible endpoint, you can point it at any backend:

```bash
# Ollama
kubectl logs <pod> | kubectl-loginsight --model qwen2.5:7b --base-url http://localhost:11434/v1

# OpenAI
kubectl logs <pod> | kubectl-loginsight --model gpt-4o --base-url https://api.openai.com/v1 --api-key sk-...
```

## Large logs

If logs exceed `--max-tokens`, the oldest lines are dropped and a warning is printed to stderr:

```
⚠  Logs truncated to 8192 tokens (oldest lines removed)
```

Use `kubectl logs --tail=200 <pod>` to limit log output before piping if needed.

**Baseline context setup:** LM Studio loaded with 8192 context (Model Settings → Context Length), Thinking mode is disabled by default to improve speed.

## Testing

Run the unit tests:
```bash
go test ./...
```

End-to-end smoke test with a sample log file:
```bash
# 1. Start LM Studio, load qwen/qwen3.5-9b GGUF, and start the local server
#    (Server runs at http://localhost:1234/v1 by default)

# 2. Create a sample log file
cat > /tmp/test.log <<EOF
2024-01-01T10:00:00Z INFO  Starting server on :8080
2024-01-01T10:01:00Z INFO  Connected to database
2024-01-01T10:02:00Z ERROR Failed to process request: connection refused
2024-01-01T10:02:01Z WARN  Retrying (attempt 1/3)
2024-01-01T10:02:03Z ERROR Max retries exceeded, giving up
EOF

# 3. Summarize
kubectl-loginsight --file /tmp/test.log --model qwen/qwen3.5-9b --base-url http://localhost:1234/v1

# 4. Ask a question
kubectl-loginsight --file /tmp/test.log --model qwen/qwen3.5-9b --base-url http://localhost:1234/v1 -q "what went wrong?"
```

## Example output

```
kubectl logs prometheus-metrics-prometheus-1 | kubectl loginsight -q "do you find any problem?"
⚠  Logs truncated to 8192 tokens (oldest lines removed)


Yes, I found several problems in these logs. Here's my analysis:

## **Critical Issues:**

### 1. **Rule Evaluation Failures (Most Common)**
Multiple recording rules are failing repeatedly with the same error:

"vector contains metrics with the same labelset after applying rule labels"


**Affected Rules:**
| Rule Group | Metric Name | Example Expression |
|------------|-------------|-------------------|
| node_timex_offset_seconds | `node_timex_offset_seconds` | `record: node_timex_offset_seconds` |
| node_network_receive_errs_total | `node_network_receive_errs_total` | Simple record rule |
| node_filesystem_free_bytes | `node_filesystem_free_bytes` | `record: node_filesystem_free_bytes` |
| node_filesystem_readonly | `node_filesystem_readonly` | Simple record rule |
| node_filesystem_avail_bytes | `node_filesystem_avail_bytes` | Uses `group_left (node)` with `pod_info` |
| node_filesystem_size_bytes | `node_filesystem_size_bytes` | Uses `group_left (node)` with `pod_info` |
| node_network_transmit_errs_total | `node_network_transmit_errs_total` | Simple record rule |
| node_timex_maxerror_seconds | `node_timex_maxerror_seconds` | Simple record rule |

### 2. **Data Ingestion Errors**
```
"Error on ingesting results from rule evaluation with different value but same timestamp"
num_dropped=3
```
This affects: `coredns_dns_responses_total:sum_rate2m` (3 data points dropped)

## **Root Cause:**
The recording rules are adding a common label (`grafanastack: xxx`) to metrics that already have overlapping label sets, causing Prometheus to reject them as they can't uniquely identify the resulting vectors.

## **Impact:**
- ❌ Rules NOT executing successfully
- ❌ Metrics data will be missing in Prometheus
- ❌ Grafana dashboards may show gaps or incorrect aggregations

## **Recommended Fix:**
1. Review each recording rule file at `/etc/prometheus/rules/...`
2. Add `__name__` label to ensure uniqueness OR use different metric names
3. Consider adding additional distinguishing labels before the common labels are applied
4. Verify that source metrics don't have conflicting label combinations
⏱  53.8s
```
