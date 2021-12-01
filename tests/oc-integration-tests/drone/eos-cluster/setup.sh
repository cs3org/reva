# standard bash error handling
set -o errexit;
set -o pipefail;
set -o nounset;
# debug commands
set -x;

kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONFIG_FILE" --wait 1m

#replace localhost or 0.0.0.0 in the kubeconfig file with "docker", in order to be able to reach the cluster through the docker service
sed -i -E -e 's/localhost|0\.0\.0\.0/'"$CLUSTER_HOST"'/g' ${KUBECONFIG}

kubectl wait node --all --for condition=ready

# set up local image registry (https://kind.sigs.k8s.io/docs/user/local-registry/)
docker run \
    -d --restart=always -p "0.0.0.0:5000:5000" --name "kind-registry" \
    registry:2

docker network connect "kind" "kind-registry"

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "docker:5000"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
