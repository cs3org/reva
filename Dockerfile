# Copyright 2018-2019 CERN
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# In applying this license, CERN does not waive the privileges and immunities
# granted to it by virtue of its status as an Intergovernmental Organization 
# or submit itself to any jurisdiction.

FROM golang:1.11

ENV GO111MODULE=on
WORKDIR /go/src/github/cernbox/reva
COPY . .
RUN GO111MODULE=off make deps
RUN make
WORKDIR /go/src/github/cernbox/reva/cmd/revad
RUN go install
RUN mkdir -p /etc/revad/
RUN cp /go/src/github/cernbox/reva/cmd/revad/revad.toml /etc/revad/revad.toml
EXPOSE 9998
EXPOSE 9999
CMD ["/go/bin/revad", "-c", "/etc/revad/revad.toml", "-p", "/var/run/revad.pid"]
