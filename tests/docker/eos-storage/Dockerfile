FROM gitlab-registry.cern.ch/dss/eos/eos-ci:5.1.25

COPY scripts/eos-run.sh /mnt/scripts/eos-run.sh
COPY sssd/sssd.conf /etc/sssd/sssd.conf

RUN ulimit -n 1024000 && yum install -y sssd sssd-client

RUN chmod 0600 /etc/sssd/sssd.conf && chown root:root /etc/sssd/sssd.conf

ENTRYPOINT /mnt/scripts/eos-run.sh