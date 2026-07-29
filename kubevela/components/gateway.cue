// SPDX-License-Identifier: Apache-2.0

// Gateway Trait Definition — Service + Ingress
// Adapted from fern-platform patterns for KubeVela OAM
// Usage: vela def apply kubevela/components/gateway.cue
//
// NOTE: This trait is designed for local k3d development using Traefik Ingress.
// For production deployments, model your own gateway/ingress definition based
// on your cluster's ingress controller (e.g., Nginx, Kong, ALB, etc.).

"gateway": {
	alias: ""
	annotations: {}
	attributes: appliesToWorkloads: ["*"]
	description: "Exposes a component via Service and Ingress (default: Traefik for k3d)"
	labels: {}
	type: "trait"
}

template: {
	outputs: {
		service: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: context.name
			spec: {
				selector: "app.oam.dev/component": context.name
				ports: [
					for k, v in parameter.http {
						{
							port:       v
							targetPort: v
							protocol:   "TCP"
							name:       "port-\(v)"
						}
					},
				]
			}
		}

		ingress: {
			apiVersion: "networking.k8s.io/v1"
			kind:       "Ingress"
			metadata: {
				name: context.name
				annotations: {
					"kubernetes.io/ingress.class": parameter.class
				}
			}
			spec: {
				if parameter.tls != _|_ {
					tls: parameter.tls
				}
				rules: [{
					host: parameter.domain
					http: paths: [
						for k, v in parameter.http {
							{
								path:     k
								pathType: "Prefix"
								backend: service: {
									name: context.name
									port: number: v
								}
							}
						},
					]
				}]
			}
		}
	}

	parameter: {
		// Hostname for the Ingress rule
		domain: string

		// Map of URL path prefix to container port
		http: [string]: int

		// Ingress controller class (default: traefik for k3d)
		class: *"traefik" | string

		// Optional TLS configuration
		tls?: [...{
			secretName: string
			hosts: [...string]
		}]
	}
}
