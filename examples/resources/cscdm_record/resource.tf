resource "cscdm_record" "www_example_com" {
  zone  = "example.com"
  type  = "A"
  key   = "www"
  value = "127.0.0.1"
  ttl   = 300
}
