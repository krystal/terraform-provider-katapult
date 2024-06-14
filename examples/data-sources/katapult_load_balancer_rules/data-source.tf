# Get load balancer rules by ID
data "katapult_load_balancer_rules" "db-replicas" {
  load_balancer_id = "lb_tBDxLKy1r0OR4Wjl"
}
