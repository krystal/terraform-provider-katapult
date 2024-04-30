# Use a seperate `katapult_load_balancer_rule` resource to add rules to an
# existing `katapult_load_balancer`.
resource "katapult_load_balancer" "my_lb" {
  name = "vm"
}

resource "katapult_load_balancer_rule" "my_rule" {
	load_balancer_id = katapult_load_balancer.my_lb.id
	destination_port = 8080
	listen_port = 80
	protocol = "HTTP"
	passthrough_ssl = false
}

# Complete example of a load balancer rule 

resource "katapult_load_balancer_rule" "complete_rule" {
	load_balancer_id = katapult_load_balancer.my_lb.id
	destination_port = 8443
	listen_port = 443
	protocol = "HTTPS"
	algorithm = "sticky"
	passthrough_ssl = true
	backend_ssl = true
	proxy_protocol = true
	check_enabled = true
	check_fall = 2
	check_interval = 20
	check_http_statuses = "2"
	check_path = "/healthz"
	check_rise = 2
	check_timeout = 5
	check_protocol = "HTTP"
}