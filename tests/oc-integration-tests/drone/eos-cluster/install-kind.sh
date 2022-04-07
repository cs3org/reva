# standard bash error handling
set -o errexit;
set -o pipefail;
set -o nounset;
# debug commands
set -x;

KIND=v0.11.0
KUBECTL=v1.21.0
HELM=v3.8.1

install(){
  wget -O /usr/local/bin/$1 $2
  chmod +x /usr/local/bin/$1
}

install_helm(){
  wget $1
  tar -zxvf helm-${HELM}-linux-amd64.tar.gz
  mv linux-amd64/helm /usr/local/bin/helm
  rm -rf helm-${HELM}-linux-amd64.tar.gz linux-amd64
  chmod +x /usr/local/bin/helm
}

# installing kind
install "kind" "https://github.com/kubernetes-sigs/kind/releases/download/${KIND}/kind-linux-amd64"

# installing kubectl
install "kubectl" "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl"

# installing helm
install_helm "https://get.helm.sh/helm-${HELM}-linux-amd64.tar.gz"

#installing nginx ingress controller
#https://kind.sigs.k8s.io/docs/user/ingress/#ingress-nginx
#kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
#
#kubectl wait --namespace ingress-nginx \
#  --for=condition=ready pod \
#  --selector=app.kubernetes.io/component=controller \
#  --timeout=90s
