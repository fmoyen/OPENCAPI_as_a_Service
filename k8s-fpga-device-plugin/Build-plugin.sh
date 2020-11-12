#!/bin/bash

echo
echo "For generating the plugin daemonset this script $0 will use following steps:"
echo
echo " - After edit the code, run the bash script build"
echo " - Run docker command docker build -t [docker-repository]:[docker-tag] . to create a docker image of the new daemonset"
echo " - Check the new generating daemonset docker image with command docker images"
echo " - Push the new docker image to a public dockerhub repository,  if needed to push/pull from a private docker repository you can reference https://kubernetes.io/docs/concepts/containers/images/#using-a-private-registry"
echo " - Change the containers image of fpga-device-plugin.yml to the new generating docker images (already done)"
echo " - Use command kubectl create -f fpga-device-plugin.yml to create the new daemonset"
echo

docker_repository="fmoyen/xilinx-fpga-device-plugin-ppc64"
docker_tag="v1.1"
ScriptDir=`realpath $0`
ScriptDir=`dirname $ScriptDir`

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
