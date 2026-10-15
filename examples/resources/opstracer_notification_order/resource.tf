resource "opstracer_notification_order" "org" {
  enabled = true
  tiers = [
    { channels = ["slack", "push"] },
    { channels = ["sms"], delay_seconds = 120 },
    { channels = ["voice"], delay_seconds = 300 },
  ]
}
