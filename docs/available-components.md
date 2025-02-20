# Table list of the deployed components

| Name                      | Description                                                                                                                 | Enabled |
|---------------------------|-----------------------------------------------------------------------------------------------------------------------------|---------|
| assisted-service          | Installs OpenShift with minimal infrastructure prerequisites and comprehensive pre-flight validations.                      | True    |
| cluster-lifecycle         | Provides cluster management capabilities for {ocp-short} and {product-title-short} hub clusters.                            | True    |
| cluster-manager           | Manages various cluster-related operations within the cluster environment.                                                  | True    |
| cluster-proxy-addon       | Automates the installation of apiserver-network-proxy on both hub and managed clusters using a reverse proxy server.        | True    |
| console-mce               | Enables the {mce-short} console plug-in.                                                                                    | True    |
| discovery                 | Discovers and identifies new clusters within the {ocm}.                                                                     | True    |
| hive                      | Provisions and performs initial configuration of {ocp-short} clusters.                                                      | True    |
| hypershift                | Hosts OpenShift control planes at scale with cost and time efficiency, and cross-cloud portability.                         | True    |
| hypershift-local-hosting  | Enables local hosting capabilities for within the local cluster environment.                                                | True    |
| local-cluster             | Enables the import and self-management of the local hub cluster where the {mce-short} is deployed.                          | True    |
| managedserviceaccount     | Synchronizes service accounts to managed clusters, and collects tokens as secret resources to give back to the hub cluster. | True    |
| server-foundation         | Provides foundational services for server-side operations within the cluster environment.                                   | True    |