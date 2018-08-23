# Ansible Operator: An Operator Backed by Ansible

### Project Status: pre-alpha

The project is currently pre-alpha, and it is expected that breaking changes to the API will be made in the upcoming releases.

## Example of how to use the Ansible Operator

### Quickstart
To quickly get started you can use [minikube][1] to start a cluster. Then you
can follow the below commands to deploy an operator, create an custom resource
and have your deployed operator react to that resource.

```bash
$ kubectl create -f example/deploy/rbac.yaml
$ kubectl create -f example/deploy/crd.yaml
$ kubectl create -f example/deploy/operator.yaml
```

After you run these three commands you should have a running operator pod.

```bash
$ kubectl get pods
NAME                                READY     STATUS    RESTARTS   AGE
ansible-operator-686fff7889-whfdf   1/1       Running   0          38s
```

Next, you should create a custom resource.

```bash
$ kubectl create -f deploy/cr.yaml
```

After this is created you should see a new pod.

```bash
$ kubectl get pods
NAME                                READY     STATUS    RESTARTS   AGE
ansible-operator-686fff7889-whfdf   1/1       Running   0          2m
test-5b5d4b7cdf-2p9d9               1/1       Running   0          0s
```

You should also notice that the logs of the created resource contain a default
message.

```bash
$ kubectl logs -f test-5b5d4b7cdf-2p9d9
hello world
hello world
hello world
```

To change this message you can edit the custom resource by running
`$ kubectl edit database example`, and add message to the spec. This should
cause a new pod to run.

```bash
$ kubectl get pods
NAME                                READY     STATUS        RESTARTS   AGE
ansible-operator-686fff7889-whfdf   1/1       Running       0          6m
test-5b5d4b7cdf-2p9d9               1/1       Terminating   0          4m
test-88b77fc65-sjjf5                1/1       Running       0          22s
```

And you should be able to see the new log message.

```bash
$ kubectl logs -f test-88b77fc65-sjjf5
new log message!
new log message!
new log message!
```
### Run Ansible Operator locally

For development, it can be convenient to run the operator locally instead of in
the cluster.

```
mkdir -p /opt/anisble/roles
cp example/config.yaml /opt/ansible/
cp example/playbook.yaml /opt/anisble/
cp -a example/busybox /opt/ansible/roles/
```

Ensure that ansible, ansible-runner (>= 1.1.0), and ansible-runner-http are
installed. Consider using a python virtualenv. If you run the operator in a
shell with an active virtualenv, that will be propagated to ansible-runner and
ansible.

### Run

To run this operator locally, you can do the following:

1. Start a kubernetes cluster, possibly with [minikube][1]
2. `kubectl create -f deploy/crd.yaml`
3. `operator-sdk up local`
4. `kubectl create -f deploy/cr.yaml`

You should then see the operator creating resources in response to the CR's creation.


## More Detailed Explanation

#### Ansible Operator Base Image

It is an CentOS based ansible-runner image, with the operator installed.  


This image should be used as a base image. An example of this can be found [here](example/Dockerfile)

The Operator expects a config file to be copied into the container at a predefined location: /opt/ansible/config.yaml

Example:
```Dockerfile
COPY config.yaml /opt/ansible/config.yaml
```

The Config file format is yaml and is an array of objects. The object has mandatory fields:
	version: The version of the Custom Resource that you will be watching.
	group: The group of the Custom Resource that you will be watching.
	kind: The kind of the Custom Resource that you will be watching.
	path:  This is the path to the playbook that you have added to the container. This playbook is expected to be simply a way to call roles.
  name: is an identifier for this combination of gvk(group, version, kind) and the path to the playbook.
```yaml
---
- name: Database
  version: v1alpha1
  group: app.example.com
  kind: Database
  path: /opt/ansible/roles/busybox/playbook.yaml
```

The operator expects that the ansible
* can handle extra vars to take parameters from the spec of the CRD
* that it is idempotent
* should be expected to be called often and without changes

#### Deploying your new Ansible Operator.

To deploy your ansible operator you will need to do 3 things.
1. Setup the RBAC permissions for the service account that the operator will run as.
2. Deploy the CRD into the cluster.
3. Deploy the operator deployment into the cluster.

##### RBAC Permissions

RBAC is the way to define permissions for a user/service account in kubernetes. The permissions in the example do two different things; create a Role and create a Role Binding. In this example we grant access to many of the “core” resources (pods,secrets,services) as well as the apps resources (deployments…). The third thing this role grants access to is the group: “app.example.com”. This is where you should state your group that you are using for the CRD. Here is the example that will help you run the operator in [minikube][1].

```yaml
kind: Role
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: app-operator
rules:
- apiGroups:
  - app.example.com
  resources:
  - "*"
  verbs:
  - "*"
- apiGroups:
  - ""
  resources:
  - pods
  - services
  - endpoints
  - persistentvolumeclaims
  - events
  - configmaps
  - secrets
  verbs:
  - "*"
- apiGroups:
  - apps
  resources:
  - deployments
  - daemonsets
  - replicasets
  - statefulsets
  verbs:
  - "*"
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1beta1
metadata:
  name: default-account-app-operator
subjects:
- kind: ServiceAccount
  name: default
roleRef:
  kind: Role
  name: app-operator
  apiGroup: rbac.authorization.k8s.io
```

##### Install the CRD into the Cluster
The CRD or Custom Resource Definition is a key extension point in kubernetes. Here you define a new resource type, and Kubernetes will handle saving and persisting this resource definition. Here is some documentation on CRDs: https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/
An example below will work with the busybox role and rbac roles above.
```yaml
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: databases.app.example.com
spec:
  group: app.example.com
  names:
    kind: Database
    listKind: DatabaseList
    plural: databases
    singular: database
  scope: Namespaced
  version: v1alpha1
```

Some things to note are that CRDs have two sections, the Spec section and the Status section. The Spec is used by the user creating the CR (custom resource). This is where a user should define the parameters for the role. The Status field is used by the operator to handle the “state” of the resource. This should not be touched by the user. There are two other fields on a CR the ObjectMeta and TypeMeta which all kubernetes objects share.

##### Operator Deployment
To deploy the operator, you should create this to manage the operator pod. The below deployment with the above code should start a running operator! Backed by Ansible!

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ansible-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      name: ansible-operator
  template:
    metadata:
      labels:
        name: ansible-operator
    spec:
      containers:
        - name: ansible-operator
          image: quay.io/water-hole/busybox-ansible-operator
          command:
          - ansible-operator
          imagePullPolicy: Always
          env:
            - name: WATCH_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace


To create a Custom Resource here is an example:
apiVersion: "app.example.com/v1alpha1"
kind: "Database"
metadata:
  name: "example"
spec:
  message: hello world 2
```

[1]: https://kubernetes.io/docs/setup/minikube/
