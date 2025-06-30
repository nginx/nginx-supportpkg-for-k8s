.PHONY: nginx-utils build install
build:
	go build -o cmd/kubectl-nginx_supportpkg

nginx-utils:
	docker buildx build --build-context project=nginx-utils --platform linux/amd64 -t nginx-utils -f nginx-utils/Dockerfile .

install: build
	sudo cp cmd/kubectl-nginx_supportpkg /usr/local/bin