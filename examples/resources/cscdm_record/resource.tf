resource "cscdm_record" "www_example_com" {
  zone  = "example.com"
  type  = "A"
  key   = "www"
  value = "127.0.0.1"
  ttl   = 300

  # optional: sent to CSC as "changeId" and "comments" on the zone edit
  change_id = "example change ID"
  comments  = "some example comments"
}
