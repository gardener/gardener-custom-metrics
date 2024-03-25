Due to dependency issues in the past, the `github.com/gardener/gardener` dependency is not imported directly via go.mod. Instead the required files and packages are copied under `./third_party/gardener/gardener`.
However, this will be soon fixed as part of https://github.com/gardener/gardener-custom-metrics/issues/5.

The revision of the used `github.com/gardener/gardener` is v1.90.5.
