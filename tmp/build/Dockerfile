FROM alpine:3.6

RUN adduser -D ansible-operator
USER ansible-operator

ADD tmp/_output/bin/ansible-operator /usr/local/bin/ansible-operator
