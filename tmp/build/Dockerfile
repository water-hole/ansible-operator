FROM ansible/ansible-runner

RUN pip install --upgrade setuptools
RUN pip install openshift ansible-runner-http

RUN echo "localhost ansible_connection=local" > /etc/ansible/hosts \
    && echo '[defaults]' > /etc/ansible/ansible.cfg \
    && echo 'roles_path = /opt/ansible/roles' >> /etc/ansible/ansible.cfg \
    && echo 'library = /usr/share/ansible/openshift' >> /etc/ansible/ansible.cfg

# install operator binary
ADD tmp/_output/bin/ansible-operator /usr/local/bin/ansible-operator
