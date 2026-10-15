data "opstracer_user" "alice" {
  email = "alice@example.com"
}

resource "opstracer_team" "backend" {
  name            = "Backend"
  member_user_ids = [data.opstracer_user.alice.id]
}
