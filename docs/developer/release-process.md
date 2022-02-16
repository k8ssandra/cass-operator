# How to make a release

* Make a `release` branch (such as 1.8.x)
* Run all int tests
* Update `CHANGELOG.md`
* Update `README.md` links/references to the right version in URLs like `https://.../k8ssandra/cass-operator/v1.8.0/...`
* Update Kustomize newTag(s) to future tag value and set correct values to image_config.yaml
* Create a tag and watch that release process completes in the github actions

# How to release Red Hat certified bundles

* Upload images to the Red Hat using the connect.redhat.com
  * If the images are built using ubi8-micro, you need to make a support ticket to ask them for a waiver since their validation process is broken
  * Include oisp, project_id, sha256 of the image etc.
  * Proceed only after you've received OK from them
* Create bundle with the new SHA256 image: (make VERSION=1.9.0 IMG=registry.connect.redhat.com/datastax/cass-operator@sha256:f14b6b217ecf4f6b3dc1b43210ed51d6be56d70e5bbc5444861df73934631d3c bundle)
* Modify bundle's imageConfig to use the SHA256 image of system-logger
* Verify CSV's metadata.name to be cass-operator.v<version>
* Ensure yamllint passes
  * Line changes are not validated by RH, and also some indentation mistakes are fine (array indentation starts from same pos is fine)
* Verify version is set correctly (and not v1.9.0-dev.ac96a72-20220209 etc)
* Add spec/relatedImages to CSV
```
spec:
  relatedImages:
    - name: cass-operator
      image: registry.connect.redhat.com/datastax/cass-operator@sha256:f14b6b217ecf4f6b3dc1b43210ed51d6be56d70e5bbc5444861df73934631d3c
    - name: system-logger
      image: registry.connect.redhat.com/datastax/system-logger@sha256:33e75d0c78a277cdc37be24f2b116cade0d9b7dc7249610cdf9bf0705c8a040e
```
* Ensure the post-release and pre-release scripts are correct if you've made any changes to the structure of the project

## To improve release processing

This process starts in the github actions workflow dispatch. It calls the pre, release, post parts

### pre-release

#### scripts/pre-release-process.sh
* Modify CHANGELOG automatically to match the tag (for unreleased)
* Modify README to include proper installation refs (keep them also?)
* Modify config/manager/kustomization.yaml to have proper newTag for cass-operator
* Modify config/manager/image_config.yaml to have proper version for server-system-logger

#### GHA
* Commit changes

### release-github-action

* Copy the unreleased notes to Github releases
* Add Github packages upload for docker images

### post-release

#### scripts/post-release-process.sh
* Return config/manager/kustomization.yaml to :latest
* Return config/manager/image_config.yaml to :latest

#### GHA
* Commit changes

#### Manual work (for now)
* Add also new ## unreleased after the tagging (post-release-process.sh)
* Modify Makefile for the next VERSION in line

