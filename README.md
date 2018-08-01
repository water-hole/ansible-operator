## POC Code for a operator backed by ansible

## Get Started

### Setup

```
mkdir -p /opt/anisble/roles
cp example/config.yaml /opt/ansible/
cp example/playbook.yaml /opt/anisble/
cp example/busybox /opt/ansible/roles/
```

### Run

To run this operator, you can do the following:

1. Start a kubernetes cluster, possibly with minikube
2. `kubectl create -f deploy/crd.yaml`
3. Start the operator using the operator-sdk's documentation. To run locally, try `operator-sdk up local`
4. `kubectl create -f deploy/cr.yaml`

You should then see the operator creating resources in response to the CR's creation.
