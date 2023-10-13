FROM gitlab-registry.cern.ch/dss/eos/eos-ci:5.1.25

RUN ulimit -n 1024000 && yum install -y sssd sssd-client

COPY --chown=root:root --chmod=0600 sssd/sssd.conf /etc/sssd/sssd.conf
COPY --chmod=0700 scripts/eos-init.sh /eos-init.sh

HEALTHCHECK --timeout=1s --interval=5s CMD /usr/bin/eos version

ENTRYPOINT [ "/eos-init.sh" ]
