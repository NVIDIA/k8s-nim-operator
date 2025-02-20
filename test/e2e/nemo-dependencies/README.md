# NeMo Dependencies

## Prerequisites

- Kubernetes Cluster with Default storage class

Purpose of this project is to install NeMo microservice dependencies and not itended for production environment.

Please clone the repo with below command 

```
git clone https://github.com/NVIDIA/k8s-nim-operator.git
```

## Installing Dependencies

Enter into `k8s-nim-operator/deployments/nemo-dependencies` directory

```
cd k8s-nim-operator/deployments/nemo-dependencies
```

Update the `values.yaml` with required NeMo Services to install under `install`

`Example:`

``` yaml
install:
  customizer: no
  datastore: no
  embedding: no
  ranking: no
  entity_store: no
  evaluator: yes
```

Run the Anisble Playbook command to install the NeMo services

```
ansible-playbook -c local -i localhost install.yaml
```

`NOTE:` At this moment it's validated on local kubernetes cluster, remote installation will be supported soon.

## Uninstalling Dependencies

```
cd nemo-depenedencies
```

Update the `values.yaml` with required NeMo Services uninstall under `uninstall` 

`Example:`

```yaml
uninstall:
  customizer: no
  datastore: no
  embedding: no
  ranking: no
  entity_store: no
  evaluator: yes
```

Run the Anisble Playbook command to uninstall the NeMo services

```
ansible-playbook -c local -i localhost uninstall.yaml
```

## Configuration of namespaces for dependencies
By default, all dependencies are installed under the single `nemo` Kubernetes namespace.
To change the namespace for all dependencies, edit the `installation_namespace` field in `values.yaml`.

For example
```yaml
installation_namespace: foobar-namespace
```

To install each dependency in its own namespace, edit the `namespace` variable under `vars` for each customization role in `install.yaml` and `uninstall.yaml`.
Alternatively, users can remove the namespace override in `install.yaml` and `uninstall.yaml`. If the overrides are removed, then each dependencies will be installed
in the namespace defined in `<microservice>/defaults/main.yml`.