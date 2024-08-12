# Changing ulimits inside the container may not be allowed.
# This container is usually called from docker compose, 
# where the ulimits are set in the docker compose file.
# If the container needs to be run manually, the following flag
# needs to be passed, else the container will run out of fd.
# docker run myeoscontainer --ulimit nofiles:1024000:1024000

FROM gitlab-registry.cern.ch/dss/eos/eos-ci:02352772.el9

COPY scripts/eos-run.sh /mnt/scripts/eos-run.sh
COPY sssd/sssd.conf /etc/sssd/sssd.conf


RUN yum install -y sssd sssd-client

RUN chmod 0600 /etc/sssd/sssd.conf && chown root:root /etc/sssd/sssd.conf

ENTRYPOINT /mnt/scripts/eos-run.sh
