data "opstracer_user" "alice" {
  email = "alice@example.com"
}

output "alice_id" {
  value = data.opstracer_user.alice.id
}
