#
# Copyright © 2019-2022 Dell Inc. or its subsidiaries. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#      http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
#
# These values should be set for running the entire test suite
# all must be valid
Portal="1.1.1.1"
Target="iqn.1992-04.com.emc:600009700bcbb70e3287017400000000"


all:check int-test

mock-test:
	go clean -cache
	go test -v -coverprofile=c.out --run=TestMock

int-test: 
	GOISCSI_PORTAL=$(Portal) GOISCSI_TARGET=$(Target)  \
		 go test -v -timeout 20m -coverprofile=c.out -coverpkg ./...

gocover:
	go tool cover -html=c.out

check:
	gofmt -d .
	golint -set_exit_status
	go vet
