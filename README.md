Rancher Harvester (WORK-IN-PROGRESS)
========
[![Build Status](https://drone-publish.rancher.io/api/badges/rancher/harvester/status.svg)](https://drone-publish.rancher.io/rancher/harvester)

New version of Rancher VM is working in progress.

## Building

`make`


## Running

`./bin/harvester`

## Install
Harvester can be installed on a Kubernetes cluster in the following ways:
- [Helm](https://github.com/rancher/harvester/tree/master/deploy/charts/harvester)
- Rancher catalog app (add this repo to the Rancher Catalog as helm v3 app)

## Documentation
Please refer to the [docs](./docs) to find out more details.

## License
Copyright (c) 2020 [Rancher Labs, Inc.](http://rancher.com)

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
