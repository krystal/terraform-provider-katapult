# Changelog

## [0.0.12](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.11...v0.0.12) (2024-06-20)


### Features

* add katapult load balancer rule resource and data sources ([#119](https://github.com/krystal/terraform-provider-katapult/issues/119)) ([987b814](https://github.com/krystal/terraform-provider-katapult/commit/987b8140aba4155b93ce4b355e328404dccec961))


### Bug Fixes

* **load-balancer:** handle removal of all resource IDs ([#132](https://github.com/krystal/terraform-provider-katapult/issues/132)) ([b4cb8bf](https://github.com/krystal/terraform-provider-katapult/commit/b4cb8bfdc9227738de27299eb66124bfff2b9618))
* **v6provider/refresh:** refreshing missing resources now clears them from state ([#133](https://github.com/krystal/terraform-provider-katapult/issues/133)) ([0ce024b](https://github.com/krystal/terraform-provider-katapult/commit/0ce024bb457d6b21724e79ece60f5cf8ea6b477c))
* **v6provider:** add plan modifier rules to reduce excessive known after apply issues ([#131](https://github.com/krystal/terraform-provider-katapult/issues/131)) ([0fcb3a0](https://github.com/krystal/terraform-provider-katapult/commit/0fcb3a0324aba924129fde6d2d5e69368e157a5c))

## [0.0.11](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.10...v0.0.11) (2024-03-07)


### Bug Fixes

* **virtual-machine:** avoid rare error when creating a virtual machine ([#125](https://github.com/krystal/terraform-provider-katapult/issues/125)) ([63bae8d](https://github.com/krystal/terraform-provider-katapult/commit/63bae8d1e2d4e452029eb835291cfd480846a214))

## [0.0.10](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.9...v0.0.10) (2023-03-23)


### Features

* **destroy:** add skip_trash_object_purge provider option ([e45ba3f](https://github.com/krystal/terraform-provider-katapult/commit/e45ba3f23b7c2bf880cf777430f191778a4dd50f))
* **file_storage_volumes:** add katapult_file_storage_volume data source ([bc4355a](https://github.com/krystal/terraform-provider-katapult/commit/bc4355afbabf1952c97327957d4e5812267ee32b))
* **file_storage_volumes:** add katapult_file_storage_volume resource ([9bb3471](https://github.com/krystal/terraform-provider-katapult/commit/9bb3471ea39bd00f939045c6dc70ae7ea50cc394))
* **file_storage_volumes:** add katapult_file_storage_volumes data source ([3bac917](https://github.com/krystal/terraform-provider-katapult/commit/3bac91701d2a6f9a1524602f10be7308cb3c756b))

## [0.0.9](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.8...v0.0.9) (2023-03-09)


### Features

* **security_groups:** add katapult_security_group data source ([d2ce789](https://github.com/krystal/terraform-provider-katapult/commit/d2ce789392b9c9530d55de6fd5ac0c9e356ee40d))
* **security_groups:** add katapult_security_group resource ([cee9f25](https://github.com/krystal/terraform-provider-katapult/commit/cee9f250f03db5b9a65c69c69283bd5ab3dcaee8))
* **security_groups:** add katapult_security_group_rule data source ([b53fc6b](https://github.com/krystal/terraform-provider-katapult/commit/b53fc6ba78f33f6cf4588acabfceeb074dcba98f))
* **security_groups:** add katapult_security_group_rule resource ([83a5f0c](https://github.com/krystal/terraform-provider-katapult/commit/83a5f0c81febad63869b096239c161bbc165e724))
* **security_groups:** add katapult_security_group_rules data source ([8c1a3d8](https://github.com/krystal/terraform-provider-katapult/commit/8c1a3d852a2a859b162e698e08067eb76f196ca1))
* **security_groups:** add katapult_security_groups data source ([e29a7ca](https://github.com/krystal/terraform-provider-katapult/commit/e29a7caf7b31f3b64441761f2544efe6e106d149))

## [0.0.8](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.7...v0.0.8) (2023-01-26)


### Features

* **deps:** change minimum supported Terraform version from 0.14.x to 1.0.x ([11bda78](https://github.com/krystal/terraform-provider-katapult/commit/11bda78609e61213a003baeb0ffb28059819d104))
* **resource/virtual_machine:** add support for setting number and size of disks during creation ([94f42ca](https://github.com/krystal/terraform-provider-katapult/commit/94f42ca0a3649dabb21c1974b92db581f3a41576))


### Bug Fixes

* **data/virtual_machine:** populate network_speed_profile field ([cf88729](https://github.com/krystal/terraform-provider-katapult/commit/cf887296ff91a05b73d2ee9937ab7e4fa225f7e5))

## [0.0.7](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.6...v0.0.7) (2023-01-26)


### Features

* **deps:** change minimum supported Terraform version from 0.14.x to 1.0.x ([11bda78](https://github.com/krystal/terraform-provider-katapult/commit/11bda78609e61213a003baeb0ffb28059819d104))
* **resource/virtual_machine:** add support for setting number and size of disks during creation ([94f42ca](https://github.com/krystal/terraform-provider-katapult/commit/94f42ca0a3649dabb21c1974b92db581f3a41576))


### Bug Fixes

* **data/virtual_machine:** populate network_speed_profile field ([cf88729](https://github.com/krystal/terraform-provider-katapult/commit/cf887296ff91a05b73d2ee9937ab7e4fa225f7e5))

### [0.0.6](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.5...v0.0.6) (2022-03-17)

### [0.0.5](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.4...v0.0.5) (2021-07-12)


### ⚠ BREAKING CHANGES

* **deps:** The provider `organization` and `data_center` attributes can no longer accept IDs. You must provide the organization sub-domain and data center permalink.

### Features

* **deps:** update to latest go-katapult client library ([e1e9e1f](https://github.com/krystal/terraform-provider-katapult/commit/e1e9e1f0e31dcb69c32ba12d0aafc4b7b8b3b198))


### Bug Fixes

* **deps:** update terraform-json package to fix state version error ([976bcb1](https://github.com/krystal/terraform-provider-katapult/commit/976bcb1c1495bbb6a2691cccbff5a84981b6e255))


### Documentation

* **readme:** improve visual styling ([08d3b1f](https://github.com/krystal/terraform-provider-katapult/commit/08d3b1f8752a4f7f0689d1e1cb138e284bbb4198))
* **readme:** update requirements to include Terraform versions later than 0.14.x ([be7f115](https://github.com/krystal/terraform-provider-katapult/commit/be7f115f497561ec48d3553b4cbdf560aa966638))

### [0.0.4](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.3...v0.0.4) (2021-04-06)


### Documentation

* **network:** add missing speed profile examples ([f0882ab](https://github.com/krystal/terraform-provider-katapult/commit/f0882abec8a668ca0ea308a505393a1672e2a53f))
* **virtual_machine:** fix disk template value in example ([00f406e](https://github.com/krystal/terraform-provider-katapult/commit/00f406e5d1a8954118e1a431423d33fffb09aa82))

### [0.0.3](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.2...v0.0.3) (2021-04-06)


### Features

* **logging:** add debug logging of VM buildspec ([7556482](https://github.com/krystal/terraform-provider-katapult/commit/7556482e5fef937ab76324e673b8ab6143f49d2c))
* **network:** add katapult_network_speed_profile data source ([9b3e087](https://github.com/krystal/terraform-provider-katapult/commit/9b3e0878732f4df84a0e7f12319ca09a8bdfe0f4))
* **network:** add katapult_network_speed_profiles data source ([0c2c3bf](https://github.com/krystal/terraform-provider-katapult/commit/0c2c3bf62275234d2a6a01b1db1fb65708b50937))
* **virtual_machine:** enable reading and setting `network_speed_profile` ([163fa9f](https://github.com/krystal/terraform-provider-katapult/commit/163fa9fd726d68b3be36e71530786c56fdcd8653))


### Bug Fixes

* **disk_template:** use get request to fetch single disk template ([4232479](https://github.com/krystal/terraform-provider-katapult/commit/4232479822d8a044baf1738dd94670f7a11745c3))


### Documentation

* **readme:** add status badges ([c0e347a](https://github.com/krystal/terraform-provider-katapult/commit/c0e347a306d83f157f965f211c7847dd78a9157a))

### [0.0.2](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1...v0.0.2) (2021-03-09)


### Features

* **virtual_machine:** add group_id to virtual machine data source ([#60](https://github.com/krystal/terraform-provider-katapult/issues/60)) ([e3541a2](https://github.com/krystal/terraform-provider-katapult/commit/e3541a29da89eaa3139085a3baa230038ec86cc8))
* **virtual_machine:** add support for managing group assignment ([7d5a098](https://github.com/krystal/terraform-provider-katapult/commit/7d5a0983c1aa7074e56324057a3af0bcb87df6ae))
* **virtual_machine_group:** add katapult_virtual_machine_group(s) data sources ([#59](https://github.com/krystal/terraform-provider-katapult/issues/59)) ([98b4ced](https://github.com/krystal/terraform-provider-katapult/commit/98b4ced57989dd7c52694075372d90576ba3dd9e))
* **virtual_machine_groups:** add resource ([#54](https://github.com/krystal/terraform-provider-katapult/issues/54)) ([eccc702](https://github.com/krystal/terraform-provider-katapult/commit/eccc7029d2c079f09dd1f6a76ebdd52faaa7bb2a))


### Documentation

* **readme:** add details about how to create a new release ([899f783](https://github.com/krystal/terraform-provider-katapult/commit/899f7830f10c8eaf2e8016fe285357cc285b5c5d))

### [0.0.1](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1-rc.4...v0.0.1) (2021-02-26)


### Features

* **provider:** initial public release

### [0.0.1-rc.4](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1-rc.3...v0.0.1-rc.4) (2021-02-26)


### Documentation

* **exxamples:** add missing example for katapult_virtual_machine_packages data source ([7abaf68](https://github.com/krystal/terraform-provider-katapult/commit/7abaf68adc9205f7b1b2f6adddf5781e27eba570))

### [0.0.1-rc.3](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1-rc.2...v0.0.1-rc.3) (2021-02-26)


### Features

* **virtual_machine_package:** add katapult_virtual_machine_package data source ([3304d1e](https://github.com/krystal/terraform-provider-katapult/commit/3304d1e6983872a276b8e688096498e3315f7568))
* **virtual_machine_packages:** add katapult_virtual_machine_packages data source ([c43d4bc](https://github.com/krystal/terraform-provider-katapult/commit/c43d4bcaf88167affac1bd5e10119ed2075731fa))


### Documentation

* update field descriptions for katapult_disk_templates and katapult_ip ([18df3ea](https://github.com/krystal/terraform-provider-katapult/commit/18df3eab945c9e4520410f726422bfbe73652602))

### [0.0.1-rc.2](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1-rc.1...v0.0.1-rc.2) (2021-02-25)


### Documentation

* **provider:** fix formatting issues ([165772f](https://github.com/krystal/terraform-provider-katapult/commit/165772fbb804c6c304484c09b844b629a0cc094f))

### [0.0.1-rc.1](https://github.com/krystal/terraform-provider-katapult/compare/v0.0.1-rc.0...v0.0.1-rc.1) (2021-02-25)


### Features

* **disk_templates:** add include_universal option to data source ([0f64ac0](https://github.com/krystal/terraform-provider-katapult/commit/0f64ac0adf0b62880fa58f977b3074e9adb41c43))


### Bug Fixes

* **virtual_machine:** avoid timing related deletion failure ([d879885](https://github.com/krystal/terraform-provider-katapult/commit/d879885642a7bdd87d80a920d010a95b34b85e17))


### Documentation

* **examples:** remove defunct examples/main.tf ([4388c99](https://github.com/krystal/terraform-provider-katapult/commit/4388c99d84e6edd977af4fad0ba52e3d874b49a8))
* **examples:** update various examples and schema descriptions ([3f8a9d7](https://github.com/krystal/terraform-provider-katapult/commit/3f8a9d7857d52da717050e94fb59ca7c36fc8dd6))
* **provider:** improve generated schema descriptions ([1ce5c14](https://github.com/krystal/terraform-provider-katapult/commit/1ce5c14111a471a3f3e536984136f3ac108e07a0))
* **readme:** add link to provider documentation on Terraform Registry ([d74c724](https://github.com/krystal/terraform-provider-katapult/commit/d74c72490906e9ff6d0b595a437230cd2dba995b))

### 0.0.1-rc.0 (2021-02-24)


### Features

* **provider:** initial public release
