package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/krystal/go-katapult/core"
)

var (
	lbRuleAlgorithms     []string
	lbRuleAlgorithmCache []string
	lbRuleAlgorithmList  = []core.LoadBalancerRuleAlgorithm{
		core.RoundRobinRuleAlgorithm,
		core.LeastConnectionsRuleAlgorithm,
		core.StickyRuleAlgorithm,
	}

	lbRuleProtocols     []string
	lbRuleProtocolCache []string
	lbRuleProtocolList  = []core.Protocol{
		core.HTTPProtocol,
		core.HTTPSProtocol,
		core.TCPProtocol,
	}
)

func loadBalancerRuleProtocols() []string {
	if lbRuleProtocolCache != nil {
		return lbRuleProtocolCache
	}

	for _, p := range lbRuleProtocolList {
		lbRuleProtocolCache = append(lbRuleProtocolCache, string(p))
	}

	return lbRuleProtocolCache
}

func loadBalancerRuleAlgorithms() []string {
	if lbRuleAlgorithmCache != nil {
		return lbRuleAlgorithmCache
	}

	for _, p := range lbRuleAlgorithmList {
		lbRuleAlgorithmCache = append(lbRuleAlgorithmCache, string(p))
	}

	return lbRuleAlgorithmCache
}

func resourceLoadBalancerRule() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceLoadBalancerRuleCreate,
		ReadContext:   resourceLoadBalancerRuleRead,
		UpdateContext: resourceLoadBalancerRuleUpdate,
		DeleteContext: resourceLoadBalancerRuleDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"load_balancer_id": {
				Type:        schema.TypeString,
				Description: "ID of load balancer to create rule on.",
				Required:    true,
			},
			"algorithm": {
				Type:     schema.TypeString,
				Optional: true,
				Description: fmt.Sprintf(
					"Algorithm used to distribute traffic between "+
						"targets. Must be one of: `%s`",
					strings.Join(loadBalancerRuleAlgorithms(), "`, `"),
				),
				Default:          core.RoundRobinRuleAlgorithm,
				DiffSuppressFunc: caseInsensitiveDiffSuppress,
				ValidateFunc: validation.StringInSlice(
					loadBalancerRuleAlgorithms(),
					true,
				),
			},
			"destination_port": {
				Type:     schema.TypeInt,
				Required: true,
				Description: "Port on your virtual machines that traffic " +
					"will be sent to.",
				ValidateFunc: validation.IntBetween(0, 65535),
			},
			"listen_port": {
				Type:     schema.TypeInt,
				Required: true,
				Description: "Port that will be publicly available on your " +
					"load balancer's IP address. All traffic received on " +
					"this port will be directed to the virtual machines " +
					"selected for the load balancer.",
				ValidateFunc: validation.IntBetween(0, 65535),
			},
			"protocol": {
				Type:     schema.TypeString,
				Required: true,
				Description: fmt.Sprintf(
					"Network protocol. Must be one of: `%s`",
					strings.Join(loadBalancerRuleProtocols(), "`, `"),
				),
				DiffSuppressFunc: caseInsensitiveDiffSuppress,
				ValidateFunc: validation.StringInSlice(
					loadBalancerRuleProtocols(),
					true,
				),
			},
			"proxy_protocol": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"certificate_ids": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"backend_ssl": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"passthrough_ssl": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"healthcheck": {
				Type: schema.TypeList,
				Description: "Monitor the health of virtual machines and " +
					"ensure that only healthy machines receive traffic.",
				MaxItems: 1,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
						"protocol": {
							Type: schema.TypeString,
							Description: fmt.Sprintf(
								"Must be one of: `%s`, `%s`",
								core.HTTPProtocol,
								core.TCPProtocol,
							),
							Default:          core.HTTPProtocol,
							Optional:         true,
							DiffSuppressFunc: caseInsensitiveDiffSuppress,
							ValidateFunc: validation.StringInSlice(
								[]string{
									string(core.TCPProtocol),
									string(core.HTTPProtocol),
								},
								true,
							),
						},
						"path": {
							Type: schema.TypeString,
							Description: "HTTP request path used when " +
								"protocol is `HTTP`.",
							Optional: true,
							Default:  "/",
						},
						"interval": {
							Type: schema.TypeInt,
							Description: "Interval in seconds between each " +
								"check that is sent to each virtual machine",
							Optional: true,
							Default:  20,
						},
						"healthy": {
							Type: schema.TypeInt,
							Description: "Number of consecutive checks which " +
								"must succeed before a virtual machine is " +
								"considered to be healthy",
							Optional: true,
							Default:  2,
						},
						"unhealthy": {
							Type: schema.TypeInt,
							Description: "The number of consecutive checks " +
								"which must fail before a virtual machine " +
								"is considered to be unhealthy.",
							Optional: true,
							Default:  2,
						},
						"timeout": {
							Type: schema.TypeInt,
							Description: "Number seconds to wait for a check " +
								"to succeed before considering it a failure.",
							Optional: true,
							Default:  5,
						},
					},
				},
			},
		},
	}
}

func resourceLoadBalancerRuleCreate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	lbRef := core.LoadBalancerRef{ID: d.Get("load_balancer_id").(string)}

	proxyProtocol := d.Get("proxy_protocol").(bool)

	args := &core.LoadBalancerRuleArguments{
		DestinationPort: d.Get("destination_port").(int),
		ListenPort:      d.Get("listen_port").(int),
		ProxyProtocol:   &proxyProtocol,
	}

	algo, err := normalizeLoadBalancerRuleAlgorithm(d.Get("algorithm").(string))
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}
	args.Algorithm = algo

	protocol, err := normalizeLoadBalancerRuleProtocol(
		d.Get("protocol").(string),
	)
	if err != nil {
		diags = append(diags, diag.FromErr(err)...)
	}
	args.Protocol = protocol

	checkRaw := d.Get("healthcheck").([]interface{})
	if len(checkRaw) > 0 {
		check := checkRaw[0].(map[string]interface{})
		checkEnabled := check["enabled"].(bool)
		args.CheckEnabled = &checkEnabled

		checkProtocol, err := normalizeLoadBalancerRuleProtocol(
			check["protocol"].(string),
		)
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		}
		args.CheckProtocol = checkProtocol

		args.CheckPath = check["path"].(string)
		args.CheckInterval = check["interval"].(int)
		args.CheckFall = check["unhealthy"].(int)
		args.CheckRise = check["healthy"].(int)
		args.CheckTimeout = check["timeout"].(int)
	}

	if diags.HasError() {
		return diags
	}

	var certRefs []core.CertificateRef
	for _, rawID := range d.Get("certificate_ids").(*schema.Set).List() {
		certRefs = append(certRefs, core.CertificateRef{ID: rawID.(string)})
	}
	if len(certRefs) > 0 {
		args.Certificates = &certRefs
	}

	lbr, _, err := m.Core.LoadBalancerRules.Create(
		ctx, lbRef, args,
	)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(lbr.ID)

	return resourceLoadBalancerRuleRead(ctx, d, meta)
}

func resourceLoadBalancerRuleRead(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Id()

	lbr, resp, err := m.Core.LoadBalancerRules.GetByID(ctx, id)
	if err != nil {
		if resp != nil && resp.Response != nil && resp.StatusCode == 404 {
			d.SetId("")

			return diags
		}

		return diag.FromErr(err)
	}

	_ = d.Set("algorithm", string(lbr.Algorithm))
	_ = d.Set("destination_port", lbr.DestinationPort)
	_ = d.Set("listen_port", lbr.ListenPort)
	_ = d.Set("protocol", string(lbr.Protocol))
	_ = d.Set("proxy_protocol", lbr.ProxyProtocol)

	var certIDs []string
	for _, c := range lbr.Certificates {
		certIDs = append(certIDs, c.ID)
	}
	_ = d.Set("certificate_ids", certIDs)

	_ = d.Set("backend_ssl", lbr.BackendSSL)
	_ = d.Set("passthrough_ssl", lbr.PassthroughSSL)

	if lbr.CheckEnabled {
		check := map[string]interface{}{}
		check["enabled"] = lbr.CheckEnabled
		check["protocol"] = lbr.CheckProtocol
		check["path"] = lbr.CheckPath
		check["interval"] = lbr.CheckInterval
		check["healthy"] = lbr.CheckRise
		check["unhealthy"] = lbr.CheckFall
		check["timeout"] = lbr.CheckTimeout
		err = d.Set("healthcheck", []interface{}{check})
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		}
	}

	return diags
}

func resourceLoadBalancerRuleUpdate(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)
	var diags diag.Diagnostics

	id := d.Id()

	lbrRef := core.LoadBalancerRuleRef{ID: id}
	args := &core.LoadBalancerRuleArguments{}

	if d.HasChange("algorithmn") {
		v, err := normalizeLoadBalancerRuleAlgorithm(
			d.Get("algorithm").(string),
		)
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		} else {
			args.Algorithm = v
		}
	}
	if d.HasChange("destination_port") {
		args.DestinationPort = d.Get("destination_port").(int)
	}
	if d.HasChange("listen_port") {
		args.ListenPort = d.Get("listen_port").(int)
	}
	if d.HasChange("protocol") {
		v, err := normalizeLoadBalancerRuleProtocol(
			d.Get("protocol").(string),
		)
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		} else {
			args.Protocol = v
		}
	}
	if d.HasChange("proxy_protocol") {
		v := d.Get("proxy_protocol").(bool)
		args.ProxyProtocol = &v
	}
	if d.HasChange("certificate_ids") {
		var v []core.CertificateRef
		for _, rawID := range d.Get("certificate_ids").(*schema.Set).List() {
			v = append(v, core.CertificateRef{ID: rawID.(string)})
		}
		args.Certificates = &v
	}
	if d.HasChange("healthcheck.0.enabled") {
		v := d.Get("healthcheck.0.enabled").(bool)
		args.CheckEnabled = &v
	}
	if d.HasChange("healthcheck.0.protocol") {
		v, err := normalizeLoadBalancerRuleProtocol(
			d.Get("healthcheck.0.protocol").(string),
		)
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		} else {
			args.CheckProtocol = v
		}
	}
	if d.HasChange("healthcheck.0.path") {
		args.CheckPath = d.Get("healthcheck.0.path").(string)
	}
	if d.HasChange("healthcheck.0.interval") {
		args.CheckInterval = d.Get("healthcheck.0.interval").(int)
	}
	if d.HasChange("healthcheck.0.healthy") {
		args.CheckRise = d.Get("healthcheck.0.healthy").(int)
	}
	if d.HasChange("healthcheck.0.unhealthy") {
		args.CheckFall = d.Get("healthcheck.0.unhealthy").(int)
	}
	if d.HasChange("healthcheck.0.timeout") {
		args.CheckTimeout = d.Get("healthcheck.0.timeout").(int)
	}

	if diags.HasError() {
		return diags
	}

	_, _, err := m.Core.LoadBalancerRules.Update(ctx, lbrRef, args)
	if err != nil {
		return diag.FromErr(err)
	}

	return resourceLoadBalancerRuleRead(ctx, d, meta)
}

func caseInsensitiveDiffSuppress(
	_k, old, new string,
	_d *schema.ResourceData,
) bool {
	return strings.EqualFold(old, new)
}

func resourceLoadBalancerRuleDelete(
	ctx context.Context,
	d *schema.ResourceData,
	meta interface{},
) diag.Diagnostics {
	m := meta.(*Meta)

	lbrRef := core.LoadBalancerRuleRef{ID: d.Id()}

	_, _, err := m.Core.LoadBalancerRules.Delete(ctx, lbrRef)
	if err != nil {
		return diag.FromErr(err)
	}

	return diag.Diagnostics{}
}

func normalizeLoadBalancerRuleAlgorithm(
	input string,
) (core.LoadBalancerRuleAlgorithm, error) {
	for _, i := range lbRuleAlgorithmList {
		if strings.EqualFold(input, string(i)) {
			return i, nil
		}
	}

	return "", fmt.Errorf(
		"%s is not a valid load balancer rule algorithm",
		input,
	)
}

func normalizeLoadBalancerRuleProtocol(input string) (core.Protocol, error) {
	for _, i := range lbRuleProtocolList {
		if strings.EqualFold(input, string(i)) {
			return i, nil
		}
	}

	return "", fmt.Errorf(
		"%s is not a valid load balancer rule protocol",
		input,
	)
}
