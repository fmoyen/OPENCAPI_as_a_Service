#!/bin/bash

docker_repository="fmoyen/xilinx-fpga-device-plugin-ppc64"
ScriptDir=`realpath $0`
ScriptDir=`dirname $ScriptDir`

echo; echo "===================================================================================================="
echo "===================================================================================================="
echo "For generating the plugin daemonset this script $0 will use following steps:"
echo
echo " - Run the bash script build (after you edit the code)"
echo " - Run docker command docker build -t [docker-repository]:[docker-tag] . to create a docker image of the new daemonset"
echo " - Check the new generating daemonset docker image with command docker images"
echo " - Push the new docker image to a public dockerhub repository,  if needed to push/pull from a private docker repository you can reference https://kubernetes.io/docs/concepts/containers/images/#using-a-private-registry"
echo " - Change the containers image of fpga-device-plugin.yml to the new generating docker images (already done)"
echo " - Use command kubectl create -f fpga-device-plugin.yml to create the new daemonset"
echo
echo "This script will use the following dockerhub repository: $docker_repository"
echo

echo; echo "========================================================"
echo "Checking local architecture"
LocalArchitecture=`lscpu | grep Architecture`
echo $LocalArchitecture

if echo $LocalArchitecture | grep ppc64le >/dev/null; then 
   echo "Running on Power platform... Continuing !"
else
   echo "Not Running on Power platform... Exiting !"; echo
   exit 2
fi

echo; echo "========================================================"
echo "Getting the tag level"
echo;echo "Locally known images:"
docker images | grep $docker_repository

echo; echo -e "Which tag do you want to create ?: \c"
read docker_tag
if [[ -z $docker_tag ]]; then
   echo "You didn't provide any tag. Exiting..."; echo
   exit 1
fi

echo; echo "========================================================"
echo "Building the binary"
./build

echo; echo "========================================================"
echo "Generating the docker image ${docker_repository}:$docker_tag"
docker build -t ${docker_repository}:$docker_tag .

echo; echo "========================================================"
echo "Tagging with \"latest\" the new docker image"
docker tag ${docker_repository}:$docker_tag ${docker_repository}:latest

echo; echo "========================================================"
echo "Checking the docker image"
docker images | grep $docker_repository

echo; echo "========================================================"
echo "Pushing the docker images to the docker hub"
docker push ${docker_repository}:$docker_tag ${docker_repository}:latest
docker push ${docker_repository}:latest

echo; echo "========================================================"
echo "Next steps to be manually done with jarvice@kubclient1"
echo "cd $ScriptDir"
echo "kubectl create -f fpga-device-plugin.yml"
echo "kubectl get daemonset -A"

echo; echo "========================================================"
echo "Bye !"
echo
