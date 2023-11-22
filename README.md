# gardener-custom-metrics
[![reuse compliant](https://reuse.software/badge/reuse-compliant.svg)](https://reuse.software/)
## Overview
The `gardener-custom-metrics` component operates as a K8s API service, adding functionality to the seed kube-apiserver.
It periodically scrapes the metrics endpoints of all shoot kube-apiserver pods on the seed. It implements the K8s custom
metrics API and provides K8s metrics specific to Gardener, based on custom calculations.


## How to use this repository template
- in the files
  - `.reuse/dep5`
  - `CODEOWNERS`
  - `README.md`
- replace the following placeholders
  - `<repo name>`: name of the new repository
  - `<maintainer team>`: name of the github team in [gardener teams](https://github.com/orgs/gardener/teams)
    defining maintainers of the new repository.
    If several repositories share a common topic and the same
    set of maintainers they can share a common maintainer team
- set the repository description in the "About" section of your repository
- remove this section from this `README.md`
- ask [@msohn](https://github.com/orgs/gardener/people/msohn) or another
  [owner of the gardener github organisation](https://github.com/orgs/gardener/people?query=role%3Aowner)
  - to double-check the initial content of this repository
  - to create the maintainer team for this new repository
  - to make this repository public
  - protect at least the master branch requiring mandatory code review by the maintainers defined in CODEOWNERS
  - grant admin permission to the maintainers team of the new repository defined in CODEOWNERS

## UNDER CONSTRUCTION
