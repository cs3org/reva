This tutorial shows how to deploy locally Reva and Phoenix on a Kubernetes cluster as a helm chart.

####1. Install *kubectl* and *minikube*
````
https://kubernetes.io/docs/tasks/tools/install-kubectl/
https://kubernetes.io/docs/tasks/tools/install-minikube/
````
####2. Clone the repository
````
git clone https://github.com/cs3org/reva
````
####3. Launch *minikube*
````
minikube start
````
####4. Switch to your minikube's docker*
````
eval $(minikube docker-env) 
````
In case of using Windows:
````
minikube docker-env
````
####5. Build images
````
cd reva/
docker build -t reva -f docs/content/en/docs/Tutorials/kubernetes/Dockerfile.revad .
docker build -t phoenix -f docs/content/en/docs/Tutorials/kubernetes/Dockerfile.phoenix .
````
####6. Enable *helm*
````
minikube addons enable helm-tiller
````
####7. Run *helm* chart
````
cd docs/content/en/docs/Tutorials/kubernetes
helm install reva-phoenix ./reva-phoenix
````
####8. Access *phoenix* login page in browser
````
minikube service phoenix-svc
````