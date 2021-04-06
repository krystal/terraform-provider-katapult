# Changelog

All notable changes to this project will be documented in this file. See [standard-version](https://github.com/conventional-changelog/standard-version) for commit guidelines.

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
