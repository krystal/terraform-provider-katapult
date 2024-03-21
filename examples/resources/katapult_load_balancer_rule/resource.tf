resource "katapult_load_balancer" "my_lb" {
  name = "vm"
  external_rules = true
}

resource "katapult_load_balancer_rule" "my_rule" {
	load_balancer_id = katapult_load_balancer.my_lb.id
	destination_port = 8080
	listen_port = 80
	protocol = "HTTP"
	passthrough_ssl = false
}