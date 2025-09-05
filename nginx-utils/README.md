# Introduction
The `kubectl` command of kubernetes offers a `debug` sub-command to investigate pods running (or crashing) on a node using ephemeral debug containers.  
The `nginx-utils` image can be spun into a debug container that includes various tools such as curl, tcpdump, iperf, netcat to name a few.  

Benefits:  
* Minimal image
* Scanned for vulnerabilities
* Includes well-known troubleshooting tools
* Ability to include custom tools  

# Usage
#### The command to start the debug container using the nginx-utils image version `ghcr.io/nginx/nginx-utils:v0.0.1-docker`:  
```
ns=v4nic-0
pod=$(kubectl get po -n $ns -o name | awk -F '/' '{print $2}')
kubectl -n $ns debug -it $pod --image=ghcr.io/nginx/nginx-utils:v0.0.1-docker --target=nginx-ingress
```

Please refer to the [nginx-utils packages page](https://github.com/nginx/nginx-supportpkg-for-k8s/pkgs/container/nginx-utils) for versions.  

--------------
#### Example usage of `api_stats.sh` found in the container:  
```
kubectl -n $ns debug -it $pod --image=ghcr.io/nginx/nginx-utils:v0.0.1-docker --target=nginx-ingress

v4nic-0-nginx-ingress-controller-854796ff97-hhkk5:~# ls
api_stats.sh     memory_stats.sh

v4nic-0-nginx-ingress-controller-854796ff97-hhkk5:~# ./api_stats.sh -h

Usage: ./api_stats.sh [-p port]

v4nic-0-nginx-ingress-controller-854796ff97-hhkk5:~# ./api_stats.sh -p 8080
API_VERSION: 9
**** /api/9/nginx ****
{
  "version": "1.27.2",
  "build": "nginx-plus-r33-p2",
  "address": "127.0.0.1",
  "generation": 8,
....
}
```  

# Building
Please go the root directory of this project, `nginx-supportpkg-for-k8s` and run the command:
```
make nginx-utils
```