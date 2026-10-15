data "opstracer_user" "alice" {
  email = "alice@example.com"
}

# Adopts an existing member. Destroying this only stops managing the member.
resource "opstracer_org_member" "alice" {
  user_id  = data.opstracer_user.alice.id
  role     = "ADMIN"
  title_id = opstracer_member_title.sre.id
}
