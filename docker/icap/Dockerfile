FROM ubuntu:18.04
RUN apt-get update && \
    apt-get -y upgrade && \
    apt-get install -y c-icap libicapapi-dev clamav libc-icap-mod-virus-scan clamav-daemon

ADD data/main.cvd /var/lib/clamav/main.cvd
ADD data/daily.cvd /var/lib/clamav/daily.cvd
ADD data/bytecode.cvd /var/lib/clamav/bytecode.cvd

RUN mkdir -p /var/run/c-icap
RUN touch /var/run/c-icap/c-icap.id
RUN chown -R c-icap:c-icap /var/run/c-icap
RUN chown -R c-icap:c-icap /etc/c-icap/

RUN mkdir -p /var/run/clamav
RUN chown clamav:clamav /var/run/clamav

ADD data/c-icap.conf /etc/c-icap/c-icap.conf
ADD data/clamav_mod.conf /etc/c-icap/clamav_mod.conf
ADD data/clamd_mod.conf /etc/c-icap/clamd_mod.conf
ADD data/virus_scan.conf /etc/c-icap/virus_scan.conf
ADD data/freshclam.conf /etc/clamav/freshclam.conf

ADD entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ADD test /test

ENTRYPOINT ["/entrypoint.sh"]
