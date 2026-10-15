package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func checkTokenHint(s *terraform.State) error {
	attrs := s.RootModule().Resources["opstracer_integration.alerts"].Primary.Attributes
	if !strings.HasSuffix(attrs["routing_key"], attrs["token_hint"]) {
		return fmt.Errorf("token_hint %q does not match routing_key", attrs["token_hint"])
	}
	return nil
}

func TestIntegrationsAndDataSources(t *testing.T) {
	srv := newFakeAPI(t)

	config := func(name, mapping, trigger, connName, sources string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_escalation_policy" "p" {
  name   = "p"
  levels = [{ targets = [{ type = "user", user_id = "22222222-2222-2222-2222-222222222222" }] }]
}

resource "opstracer_service" "api" {
  name                 = "api"
  escalation_policy_id = opstracer_escalation_policy.p.id
}

resource "opstracer_integration" "alerts" {
  service_id         = opstracer_service.api.id
  name               = "` + name + `"
  type               = "generic"
  rate_soft_per_min  = 100
  severity_mapping   = ` + mapping + `
  rotate_key_trigger = "` + trigger + `"
}

resource "opstracer_observability_connection" "grafana" {
  type  = "grafana"
  name  = ` + connName + `
  site  = "https://grafana.example.test"
  token = "glsa_example_token_1234"
}

resource "opstracer_observability_connection" "sentry" {
  type         = "sentry"
  site         = "https://sentry.io"
  token        = "sntrys_example_token"
  organization = "acme"
}

resource "opstracer_service_data_sources" "api" {
  service_id = opstracer_service.api.id
  sources    = ` + sources + `
}
`
	}
	pinned := `{
    grafana = { choice = "connection", connection_id = opstracer_observability_connection.grafana.id }
    sentry  = { choice = "off" }
  }`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("alerts", `{ critical = "P1" }`, "1", "null", pinned),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_integration.alerts", "status", "active"),
					resource.TestCheckResourceAttr("opstracer_integration.alerts", "default_severity", "P3"),
					resource.TestCheckResourceAttr("opstracer_integration.alerts", "rate_hard_per_min", "6000"),
					resource.TestCheckResourceAttr("opstracer_integration.alerts", "severity_mapping.critical", "P1"),
					resource.TestMatchResourceAttr("opstracer_integration.alerts", "routing_key", regexp.MustCompile(`^opst_fakekey`)),
					checkTokenHint,
					resource.TestCheckResourceAttrSet("opstracer_integration.alerts", "integration_url"),
					resource.TestCheckResourceAttr("opstracer_observability_connection.grafana", "name", "Grafana"),
					resource.TestCheckResourceAttr("opstracer_observability_connection.grafana", "is_default", "true"),
					resource.TestCheckResourceAttr("opstracer_observability_connection.grafana", "credential_hints.token", "1234"),
					resource.TestCheckResourceAttr("opstracer_service_data_sources.api", "sources.%", "2"),
					resource.TestCheckResourceAttrPair("opstracer_service_data_sources.api", "sources.grafana.connection_id", "opstracer_observability_connection.grafana", "id"),
				),
			},
			{
				Config: config("alerts-v2", "null", "2", `"grafana-prod"`, `{ grafana = { choice = "default" } }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_integration.alerts", "name", "alerts-v2"),
					resource.TestCheckNoResourceAttr("opstracer_integration.alerts", "severity_mapping.%"),
					resource.TestMatchResourceAttr("opstracer_integration.alerts", "routing_key", regexp.MustCompile(`^opst_rotatedkey`)),
					checkTokenHint,
					resource.TestCheckResourceAttr("opstracer_observability_connection.grafana", "name", "grafana-prod"),
					resource.TestCheckResourceAttr("opstracer_observability_connection.grafana", "token", "glsa_example_token_1234"),
					resource.TestCheckResourceAttr("opstracer_service_data_sources.api", "sources.%", "1"),
					resource.TestCheckResourceAttr("opstracer_service_data_sources.api", "sources.grafana.choice", "default"),
				),
			},
			{
				// Neither the routing key nor the trigger can be read back.
				ResourceName:            "opstracer_integration.alerts",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"routing_key", "rotate_key_trigger"},
			},
			{
				ResourceName:            "opstracer_observability_connection.grafana",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"token"},
			},
			{
				// An explicit `default` is not visible on import; only overrides are.
				Config: config("alerts-v2", "null", "2", `"grafana-prod"`, `{ sentry = { choice = "off" } }`),
			},
			{
				ResourceName:                         "opstracer_service_data_sources.api",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "service_id",
			},
		},
	})
}
