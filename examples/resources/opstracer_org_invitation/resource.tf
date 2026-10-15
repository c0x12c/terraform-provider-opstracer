resource "opstracer_org_invitation" "alice" {
  email    = "alice@example.com"
  role     = "MEMBER"
  team_ids = [opstracer_team.backend.id]
}

# Share the join link yourself instead of emailing it.
resource "opstracer_org_invitation" "contractor" {
  email      = "contractor@example.com"
  role       = "MEMBER"
  send_email = false
}

output "contractor_invite_url" {
  value     = opstracer_org_invitation.contractor.invite_url
  sensitive = true
}
