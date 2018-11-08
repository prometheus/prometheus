DOCKER_IMG := zipkin-ui-env
DOCKER_CMD := docker run --rm -t -v $(PWD):/app -w /app $(DOCKER_IMG)

ifeq ($(NO_DOCKER), true)
  DOCKER_CMD=
endif

all: test build 

zipkin-ui-env:
	@(if [ "$(NO_DOCKER)" != "true" ] ; then \
		echo ">> building zipkin-ui-env docker image"; \
		docker build -t $(DOCKER_IMG) .;\
	fi; )

lint: zipkin-ui-env
	@echo ">> running linter on js files"
	@$(DOCKER_CMD) npm run lint

test: zipkin-ui-env
	@echo ">> running tests"
	@$(DOCKER_CMD) npm run test

build: zipkin-ui-env lint
	@echo ">> building fonts, zipkin.min.js & zipkin.min.css"
	@$(DOCKER_CMD) npm run build

clean:
	- @docker rmi $(DOCKER_IMG)