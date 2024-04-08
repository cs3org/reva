FROM gitlab-registry.cern.ch/dss/eos/eos-ci:5.1.25

COPY scripts/eos-run.sh /mnt/scripts/eos-run.sh
COPY sssd/sssd.conf /etc/sssd/sssd.conf

# RUN ulimit -n 1024000 && yum install -y sssd sssd-client
# needs to be run from docker like this:
# docker run myeoscontainer --ulimit nofiles:1024000:1024000
# or in the ulimits directive for docker compose

RUN chmod 0600 /etc/sssd/sssd.conf && chown root:root /etc/sssd/sssd.conf

ENTRYPOINT /mnt/scripts/eos-run.sh
