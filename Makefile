TARGETS := $(shell ls --ignore=help --ignore='*.txt' scripts)

SHA512SUM_Linux_aarch64 := 781951b31e5ff018a04e755c6da7163b31a81edda61f1bed4def8d0e24229865c58a3d26aa0cc4184058d91ebcae300ead2cad16d3c46ccb1098419e3e41a016
SHA512SUM_Linux_x86_64 := d2ec27ecf9362e2fafd27d76d85a5c5b92b53aefe07cffa76bf9887db6bee07b1023cca8fc32a2c9bdd2ecfadaee71397066b41bd37c9ebbbbce09913f0884d4
SHA512SUM_Darwin_arm64 := 8a356c89ad32af1698ae8615a6e303773a8ac58b114368454d59965ec2aa8282e780d1e228d37c301ce6f87596f68bfe7f204eb5f4c019c386a58dd94153ddcf
SHA512SUM_Darwin_x86_64 := dbab05de04dda26793f4ae7875d0fba96ee54b0228e192fd40c0b2116ed345b5444047fc2e0c90cb481f28cbe0e0452bcecb268c8d074cd8615eb2f5463c30b6
SHA512SUM_Windows_x86_64 := 807aee2f68b6da35cb0885558f5cbc9a6c8747a56c7a200f0e1fcac9e2fd0da570cbb39e48b3192bd1a71805f2ab38fd19d77faebba97a89e5d9a8b430ee429e

help:
	@./scripts/help "$(MAKEFILE_LIST)" $(TARGETS)

.dapper:
	@echo Downloading dapper
	@curl -sSfL https://releases.rancher.com/dapper/v0.6.0/dapper-$$(uname -s)-$$(uname -m) > .dapper.tmp
	@CHECKSUM=$$(shasum -a 512 .dapper.tmp | awk '{print $$1}'); \
	if [ "$$CHECKSUM" != "$(SHA512SUM_$(shell uname -s)_$(shell uname -m))" ]; then \
		echo "Checksum verification failed!"; \
		exit 1; \
	fi
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

$(TARGETS): .dapper
	./.dapper $@

.DEFAULT_GOAL := default

.PHONY: $(TARGETS)
