[![OpenSSFScorecard](https://api.securityscorecards.dev/projects/github.com/nginxinc/nginx-supportpkg-for-k8s/badge)](https://scorecard.dev/viewer/?uri=github.com/nginxinc/nginx-supportpkg-for-k8s)
[![FOSSA Status](https://app.fossa.com/api/projects/custom%2B5618%2Fgithub.com%2Fnginxinc%2Fnginx-supportpkg-for-k8s.svg?type=shield)](https://app.fossa.com/projects/custom%2B5618%2Fgithub.com%2Fnginxinc%2Fnginx-supportpkg-for-k8s?ref=badge_shield)
[![Go Report Card](https://goreportcard.com/badge/github.com/nginxinc/nginx-supportpkg-for-k8s)](https://goreportcard.com/report/github.com/nginxinc/nginx-supportpkg-for-k8s)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/nginxinc/nginx-supportpkg-for-k8s?logo=go)
[![Project Status: Active – The project has reached a stable, usable state and is being actively developed.](https://www.repostatus.org/badges/latest/active.svg)](https://www.repostatus.org/#active)


# nginx-supportpkg-for-k8s

A kubectl plugin designed to collect diagnostics information on any NGINX product running on k8s. 

## Supported products

Currently, [NIC](https://github.com/nginxinc/kubernetes-ingress), [NGF](https://github.com/nginxinc/nginx-gateway-fabric) and [NGINX (OSS/NPLUS) in containers](https://github.com/nginx/nginx) are the supported products.

## Features

Depending on the product, the plugin might collect some or all of the following global and namespace-specific information:

- k8s version, nodes information and CRDs
- pods logs
- list of pods, events, configmaps, services, deployments, statefulsets, replicasets and leases
- k8s metrics
- helm deployments
- `nginx -T` output from NGINX pods

The plugin DOES NOT collect secrets or coredumps.

## Prerequisites
* Install [krew](https://krew.sigs.k8s.io), the plugin manager for kubectl command-line tool, from the [official pages](https://krew.sigs.k8s.io/docs/user-guide/setup/install/)
* Run `kubectl krew` to check the installation
* Run through some of the examples in krew's [quickstart guide](https://krew.sigs.k8s.io/docs/user-guide/quickstart/)

## Installation

### Install from krew
The `nginx-supportpkg` plugin can be found in the list of kubectl plugins distributed on the centralized [krew-index](https://sigs.k8s.io/krew-index).

To install `nginx-supportpkg` plugin on your machine:
* Run `kubectl krew install nginx-supportpkg`


### Building from source
Clone the repo and run `make install`. This will build the binary and copy it on `/usr/local/bin/`.

Verify that the plugin is properly found by `kubectl`:

```
$ kubectl plugin list
The following compatible plugins are available:

/usr/local/bin/kubectl-nginx_supportpkg
```

### Downloading the binary

Navigate to the [releases](https://github.com/nginxinc/nginx-supportpkg-for-k8s/releases) section and download the asset for your operating system and architecture from the most recent version. 

Decompress the tarball and copy the binary somewhere in your `$PATH`. Make sure it is recognized by `kubectl`:

```
$ kubectl plugin list
The following compatible plugins are available:

/path/to/plugin/kubectl-nginx_supportpkg
```

## Usage

The plugin is invoked via `kubectl nginx-supportpkg` and has two required flags:

* `-n` or `--namespace` indicates the namespace(s) where the product is running.
* `-p` or `--product` indicates the product to collect information from.


```
$ kubectl nginx-supportpkg -n default -n nginx-ingress-0 -p nic
Running job pod-list... OK
Running job collect-pods-logs... OK
Running job events-list... OK
Running job configmap-list... OK
Running job service-list... OK
Running job deployment-list... OK
Running job statefulset-list... OK
Running job replicaset-list... OK
Running job lease-list... OK
Running job k8s-version... OK
Running job crd-info... OK
Running job nodes-info... OK
Running job metrics-information... OK
Running job helm-info... OK
Running job helm-deployments... OK
Supportpkg successfully generated: nic-supportpkg-1711384966.tar.gz

```