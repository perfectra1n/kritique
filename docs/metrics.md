# Metrics

The management port serves `/metrics` alongside `/healthz` and `/readyz`.
Beyond the Go runtime, every series is prefixed `kritique_` and labelled by
tenant, installation or model, never by pull request or commit:

| Series                                                                                 | Labels                | What it counts                                                                |
| -------------------------------------------------------------------------------------- | --------------------- | ----------------------------------------------------------------------------- |
| `kritique_webhooks_total`                                                                | installation, outcome | deliveries: enqueued, skipped, ignored, ping, unauthorized, unparsable, error |
| `kritique_polls_total`, `kritique_polled_pull_requests_total`                              | installation, outcome | backstop polls and the open pull requests they handed to ingest               |
| `kritique_reviews_total`, `kritique_review_duration_seconds`                               | tenant, status        | reviews by terminal status and wall time                                      |
| `kritique_findings_total`                                                                | tenant, severity      | findings posted                                                               |
| `kritique_followups_total`                                                               | tenant, outcome       | mentions handled: answered, limited, ignored, failed                          |
| `kritique_context_chunks_total`                                                          | tenant, stage         | context chunks put in front of the model                                      |
| `kritique_index_runs_total`, `kritique_index_chunks_total`                                 | tenant, mode, status  | index runs and chunks embedded                                                |
| `kritique_runner_runs_total`, `kritique_runner_duration_seconds`                           | tenant, kind, outcome | runner Jobs: success, failed, deadline                                        |
| `kritique_lease_wait_seconds`                                                            | tenant, model         | time waiting for a model concurrency slot                                     |
| `kritique_review_snoozes_total`                                                          | tenant, model         | reviews put back on the queue because every model slot was held               |
| `kritique_model_calls_total`, `kritique_model_tokens_total`, `kritique_model_cost_usd_total` | tenant, model, role   | calls; tokens by direction (input, cached, output); provider-reported cost    |
| `kritique_config_drift`                                                                  |                       | 1 while this replica's file differs from the applied one                      |
