package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestServiceAndEscalationPolicy(t *testing.T) {
	srv := newFakeAPI(t)

	config := func(serviceName, timeout string) string {
		return providerConfig(srv.URL) + `
resource "opstracer_escalation_policy" "primary" {
  name = "backend-primary"
  levels = [
    {
      timeout_seconds = ` + timeout + `
      targets = [{ type = "schedule", schedule_id = "11111111-1111-1111-1111-111111111111" }]
    },
    {
      targets = [{ type = "user", user_id = "22222222-2222-2222-2222-222222222222" }]
    },
  ]
}

resource "opstracer_service" "checkout" {
  name                 = "` + serviceName + `"
  description          = "Checkout API"
  escalation_policy_id = opstracer_escalation_policy.primary.id
}
`
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("checkout-api", "600"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("opstracer_escalation_policy.primary", "id"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.primary", "ack_timeout_seconds", "1800"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.primary", "levels.0.timeout_seconds", "600"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.primary", "levels.1.timeout_seconds", "300"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.primary", "levels.1.targets.0.type", "user"),
					resource.TestCheckResourceAttrPair("opstracer_service.checkout", "escalation_policy_id", "opstracer_escalation_policy.primary", "id"),
				),
			},
			{
				Config: config("checkout-api-v2", "900"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("opstracer_service.checkout", "name", "checkout-api-v2"),
					resource.TestCheckResourceAttr("opstracer_escalation_policy.primary", "levels.0.timeout_seconds", "900"),
				),
			},
			{
				ResourceName:      "opstracer_service.checkout",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "opstracer_escalation_policy.primary",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
