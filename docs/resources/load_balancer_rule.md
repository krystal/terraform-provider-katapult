---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "katapult_load_balancer_rule Resource - terraform-provider-katapult"
subcategory: ""
description: |-
  
---

# katapult_load_balancer_rule (Resource)



## Example Usage

```terraform
# Use a seperate `katapult_load_balancer_rule` resource to add rules to an
# existing `katapult_load_balancer`.
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
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `destination_port` (Number)
- `listen_port` (Number)
- `load_balancer_id` (String)
- `passthrough_ssl` (Boolean)
- `protocol` (String)

### Optional

- `algorithm` (String)
- `backend_ssl` (Boolean)
- `certificates` (Attributes List) (see [below for nested schema](#nestedatt--certificates))
- `check_enabled` (Boolean)
- `check_fall` (Number)
- `check_http_statuses` (String)
- `check_interval` (Number)
- `check_path` (String)
- `check_protocol` (String)
- `check_rise` (Number)
- `check_timeout` (Number)
- `proxy_protocol` (Boolean)

### Read-Only

- `id` (String) The ID of this resource.

<a id="nestedatt--certificates"></a>
### Nested Schema for `certificates`

Required:

- `id` (String)
- `name` (String)

Optional:

- `additional_names` (List of String)

Read-Only:

- `state` (String)

