# Introduction
The `kubectl` command of Kubernetes offers a `debug` sub-command to investigate pods running (or crashing) on a node using ephemeral debug containers.  
The `nginx-utils` image can be spun into a debug container that includes various tools such as curl, tcpdump, iperf, netcat to name a few.  

Benefits:  
* Minimal image
* Scanned for vulnerabilities
* Includes well-known troubleshooting tools
* Ability to include custom tools  

# Usage
#### The command to start the debug container using the nginx-utils image version `ghcr.io/nginx/nginx-utils:v0.0.1-docker`:  
```
kubectl -n <namespace> debug -it <nic-pod-name> --image=ghcr.io/nginx/nginx-utils:latest --target=nginx-ingress
```

Please refer to the [nginx-utils packages page](https://github.com/nginx/nginx-supportpkg-for-k8s/pkgs/container/nginx-utils) for versions.  

--------------
#### Example usage of `api_stats.sh` found in the container:
The `api_stats.sh` script is designed to facilitate accessing the [/api](https://nginx.org/en/docs/http/ngx_http_api_module.html#api) endpoint to query various status information, configuring upstream server groups on-the-fly, and managing key-value pairs without the need of reconfiguring nginx.

<font color="red">NOTE</font>: The `api_stats.sh` script requires an `/api` endpoint that is listening on `loopback`, in a given `port`.
```
kubectl -n <namespace> debug -it <nic-pod-name> --image=ghcr.io/nginx/nginx-utils:latest --target=nginx-ingress

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
Please go to the root directory of this project, `nginx-supportpkg-for-k8s` and run the command:
```
make nginx-utils
```