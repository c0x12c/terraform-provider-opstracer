# terraform-provider-opstracer

Terraform provider for OpsTracer on-call configuration, built on the
[Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

| Name | What it manages |
|---|---|
| `opstracer_service` | A service and the escalation policy that pages for it |
| `opstracer_escalation_policy` | Levels of users or schedules paged until someone acks |
| `opstracer_schedule` | An on-call schedule; each `layers` entry is one rotation |
| `opstracer_team` | A team and, optionally, its members |
| `opstracer_routing_rules` | All alert routing rules of the org, in order (one per org) |
| `opstracer_notification_order` | Org-wide channel order for paging (one per org) |
| `opstracer_member_title` | A custom member title |
| `opstracer_schedule_override` | Someone covers a schedule for a time window |
| `opstracer_integration` | An alert source (Datadog, Grafana, generic webhook, ...) for a service |
| `opstracer_observability_connection` | A connection to an observability tool used for investigations |
| `opstracer_service_data_sources` | Which observability connections a service uses |
| `opstracer_mute` | Silence a whole service or integration for a time window |
| `opstracer_suppression` | Silence only matching alerts of a service for a time window |
| `opstracer_org_invitation` | Invite someone to the org |
| `opstracer_org_member` | Role and title of an existing member (never removes the member) |
| `opstracer_playbook_flow` | An investigation playbook flow |
| `data.opstracer_user` | Look up a member's user ID by email |
| `data.opstracer_service`, `_escalation_policy`, `_team`, `_schedule` | Look up an existing object by name |
| `data.opstracer_oncall` | Who is on call for a schedule, now or at a given time |

Docs: [registry.terraform.io/providers/c0x12c/opstracer](https://registry.terraform.io/providers/c0x12c/opstracer/latest/docs), generated into `docs/` from schema descriptions and `examples/`. Full setup example: [team](examples/team/main.tf)
(team, rotation, routing and notification settings).

### Behaviour worth knowing

- `opstracer_routing_rules` is authoritative: rules not in the list are deleted. Rule order is
  evaluation order.
- `opstracer_notification_order` cannot be deleted in OpsTracer; destroying it only drops it from state.
- `opstracer_team.member_user_ids` left unset means membership is managed elsewhere.
- Schedule `rotation_start_at` comes back from the API in UTC. The provider keeps your spelling
  (for example `+07:00`) as long as it is the same instant. Right after `terraform import` the first
  plan shows a cosmetic change from the UTC form; applying it is a no-op.

## Usage

```hcl
terraform {
  required_providers {
    opstracer = {
      source  = "c0x12c/opstracer"
      version = "~> 0.2.2"
    }
  }
}
```

## Authentication

The provider sends `Authorization: Bearer <token>` on every request. Configure it with:

| Argument  | Env var             | Default                                   |
|-----------|---------------------|-------------------------------------------|
| `token`   | `OPSTRACER_TOKEN`   | none (required)                           |
| `api_url` | `OPSTRACER_API_URL` | `https://api-control-plane.opstracer.ai`  |

OpsTracer has no API keys yet, so for now the token is the org-scoped access token from a browser
session. To get one on dev:

1. Log in to https://app.dev.opstracer.ai and switch to the target org.
2. In the browser console run `JSON.parse(localStorage["opstracer.session"]).accessToken`.
3. `export OPSTRACER_TOKEN=<that value>` and `export OPSTRACER_API_URL=https://api-control-plane.dev.opstracer.ai`.

The token is a short-lived JWT, so this is for development only. When API keys exist, users swap the
value of `OPSTRACER_TOKEN` and nothing in the provider changes (see below).

## Development

```bash
go test ./...          # runs the full create/update/import/destroy cycle against an in-memory fake API
go generate ./...      # regenerates docs/ from schema descriptions and examples/
```

### Releasing

Run the **Release** workflow in GitHub Actions with the version (e.g. `0.2.2`), or
`gh workflow run release.yml -f version=0.2.2`. It writes the version into the examples and docs/,
commits that to main, tags `v0.2.2` and publishes. Don't push tags by hand: the release fails if
docs/ names another version.

### Adding a resource

1. Add the API calls in `internal/client/` (one file per API area).
2. Add `internal/provider/<name>_resource.go` with schema, CRUD and import.
3. Register it in `Resources()` in `provider.go`.
4. Extend the fake API in `fake_server_test.go` and add a test step.
5. Add `examples/resources/opstracer_<name>/resource.tf` and `import.sh`, then `go generate ./...` to refresh `docs/`.

## What the API needs for Terraform

These are the asks for the OpsTracer backend. The first one blocks real use; the rest avoid
plan drift and confusing errors.

**1. API keys for machines.** Terraform and CI cannot use a browser session. Add org-scoped API keys
that:

- are accepted on the same `Authorization: Bearer` header as session tokens, so the provider does not change;
- are long-lived and can be revoked and rotated from the UI;
- carry a role (read-only or admin) like a member does;
- use a recognisable prefix (for example `otk_`) so secret scanners can detect leaks.

**2. Make the OpenAPI spec match the wire format.** The real API wraps every response in
`{"data": ...}`, wraps errors in `{"error": {"code", "message", "details"}}`, and uses snake_case
(`policy_id`, `ack_timeout_s`). The published spec shows unwrapped camelCase. Any client generated
from the spec today is wrong.

**3. Keep returning 404 for a missing object.** This already works (`service_not_found`,
`policy_not_found`). The provider relies on it to detect objects deleted outside Terraform and plan
to recreate them.

**4. Return what was sent, and return every default.** `Read` must return exactly the values that
`Create` accepted, with null kept as null. If the server normalises a value (trims, reorders) Terraform
shows a diff on every plan. Defaults the server fills (`timeout_s` = 300, `repeat_count`,
`ack_timeout_s`, `repage_after_ack`) must be documented, since the provider has to mark those
fields as computed.

**5. Every resource needs create, get-by-id, update and delete.** Gaps seen in the current spec:
`org/titles`, mutes, suppressions, schedule overrides and observability connections have no
get-by-id (the provider scans the list). The notification order has no delete or reset.

**6. Return field-level errors for every bad request.** Most validation errors are good
(`target_user_required`, `severity_invalid`). But a body that fails to deserialize returns a bare
`{"code":"error","message":"Bad Request"}` with no field, which is hard to debug from Terraform.

**7. Document the schedule model.** Things learned only by trial on dev:

- a layer's members come from `participant_groups` (one inner list per turn); sending
  `participant_groups: []` with `participant_user_ids` set creates a layer with nobody on it;
- weekdays in `restrictions.days` are 1 = Monday ... 7 = Sunday;
- empty arrays are omitted from responses (`restrictions`, `participant_groups`).

The spec also types the routing rule `matcher` as an array; the API takes and returns an object
(`severity`, `source_type`, `integration_id`, `tags`, `routing_hints`).

**8. Return plan limits as a clear error.** Free plans allow one schedule, and the API answers
`limit_reached`. That is fine; it should also be in the spec so the provider docs can mention it.

**9. Name the operations.** The spec uses generated `operationId`s (`get_16`, `create_14`). Naming
them (`getService`, `createService`) makes generated clients and docs readable.

## Release

Tag `vX.Y.Z` and run GoReleaser with `GPG_FINGERPRINT` set. It builds every OS/arch, signs
`SHA256SUMS` and attaches `terraform-registry-manifest.json`, which is what the Terraform Registry
expects. The registry namespace is the GitHub owner of this repo, so this repo publishes as
`c0x12c/opstracer`.
