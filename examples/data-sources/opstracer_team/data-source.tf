data "opstracer_team" "backend" {
  name = "backend"
}

output "backend_members" {
  value = data.opstracer_team.backend.member_user_ids
}
