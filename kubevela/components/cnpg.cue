// SPDX-License-Identifier: Apache-2.0

// CloudNativePG PostgreSQL Cluster Component Definition
// Adapted from fern-platform patterns for KubeVela OAM
// Usage: vela def apply kubevela/components/cnpg.cue

"cloud-native-postgres": {
	alias: ""
	annotations: {}
	attributes: workload: definition: {
		apiVersion: "postgresql.cnpg.io/v1"
		kind:       "Cluster"
	}
	description: "CloudNativePG operator-managed PostgreSQL cluster"
	labels: {}
	type: "component"
}

template: {
	output: {
		apiVersion: "postgresql.cnpg.io/v1"
		kind:       "Cluster"
		metadata: name: parameter.name
		spec: {
			instances: parameter.instances

			storage: size: parameter.storageSize

			bootstrap: initdb: {
				database: parameter.database
				owner:    parameter.owner
			}

			if parameter.resources != _|_ {
				resources: parameter.resources
			}

			if parameter.backup != _|_ {
				backup: parameter.backup
			}
		}
	}

	parameter: {
		// Name of the CNPG cluster
		name: string

		// Number of PostgreSQL instances (1 for dev, 3 for HA)
		instances: *1 | int

		// Persistent volume size per instance
		storageSize: *"1Gi" | string

		// Database name created during bootstrap
		database: *"app" | string

		// Owner role for the bootstrap database
		owner: *"app" | string

		// Optional CPU/memory requests and limits
		resources?: {
			requests?: {
				cpu?:    string
				memory?: string
			}
			limits?: {
				cpu?:    string
				memory?: string
			}
		}

		// Optional backup configuration (e.g., Barman object store)
		backup?: {...}
	}
}
