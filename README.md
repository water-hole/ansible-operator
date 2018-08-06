## POC Code for a operator backed by ansible

## Get Started Locally

For development, it can be convenient to run the operator locally instead of in
the cluster.

### Setup

```
mkdir -p /opt/anisble/roles
cp example/config.yaml /opt/ansible/
cp example/playbook.yaml /opt/anisble/
cp -a example/busybox /opt/ansible/roles/
```

Ensure that ansible and ansible-runner (>= 1.0.5) are installed. Consider using
a python virtualenv. In that case, add these lines to your playbook:

```
  vars:
    ansible_python_interpreter: /path/to/your/virtualenv/bin/python
```

### Run

To run this operator locally, you can do the following:

1. Start a kubernetes cluster, possibly with minikube
2. `kubectl create -f example/deploy/crd.yaml`
3. `operator-sdk up local`
4. `kubectl create -f example/deploy/cr.yaml`

You should then see the operator creating resources in response to the CR's creation.
