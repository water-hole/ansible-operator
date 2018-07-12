FROM docker.io/shurley/ansible-operator:demo

COPY busybox/ /opt/ansible/roles/busybox/
COPY playbook.yaml /opt/ansible/roles/busybox/playbook.yaml
COPY config.yaml /opt/ansible/config.yaml

RUN adduser -D ansible-operator
USER ansible-operator
