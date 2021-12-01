# standard bash error handling
set -o errexit;
set -o pipefail;
set -o nounset;
# debug commands
set -x;

KIND=v0.11.0
KUBECTL=v1.21.0

install(){
  wget -O /usr/local/bin/$1 $2
  chmod +x /usr/local/bin/$1
}

# installing kind
install "kind" "https://github.com/kubernetes-sigs/kind/releases/download/${KIND}/kind-linux-amd64"

#installing kubectl
install "kubectl" "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl"
