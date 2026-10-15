# Building blocks: GET /api/v1/investigation/playbook-flow/catalog.
resource "opstracer_playbook_flow" "database" {
  name        = "database-first"
  description = "Check the database before anything else"
  graph = jsonencode({
    nodes = [
      { id = "incident", type = "incident", position = { x = 0, y = 0 } },
      { id = "db", type = "playbook", playbook = "database", position = { x = 250, y = 0 }, values = { env = "prod" } },
      { id = "end", type = "end", position = { x = 500, y = 0 } },
    ]
    edges = [
      { source = "incident", source_handle = "start", target = "db", target_handle = "after" },
      { source = "db", source_handle = "done", target = "end", target_handle = "after" },
    ]
  })
}
