# How to make a release

These instructions apply to the current master, some branches might have older scripts and require more manual work. Please read the instructions from that branch and do your diligence.

After ensuring the work for the release is done and integration tests work:

Run the following sequence, doing the small manual steps when required (until automated). There is no need to make a release branch as we want to maintain documentation in the master also.

Ensure the post-release and pre-release scripts are correct if you've made any changes to the structure of the project since the last release.

The steps required (assuming releasing v1.13.0), doing from master (replace git commands with correct target remote names):

```
scripts/pre-release-process.sh v1.13.0
git push upstream master
git push upstream v1.13.0
```

```
scripts/post-release-process.sh
git push upstream master
```

If you wish to prepare for the next patch release (such as when releasing from a branch), use: ``PATCH_RELEASE=true scripts/post-release-process.sh``. You can do this after branching the tag but before applying other changes.

The github action will create docker images and push them to DockerHub as well as Red Hat Connect repository. Verify it completes successfully.

# Steps and what they do

## pre-release

### scripts/pre-release-process.sh
* Modify CHANGELOG automatically to match the tag (for unreleased)
* Modify README to include proper installation refs (keep them also?)
* Modify config/manager/kustomization.yaml to have proper newTag for cass-operator
* Modify config/manager/image_config.yaml to have proper version for server-system-logger
* Git add all the modified files
* Git commit and create new annotated tag

### Manual
* Push the tag and changes

## post-release

### scripts/post-release-process.sh
* Return config/manager/kustomization.yaml to :latest
* Return config/manager/image_config.yaml to :latest
* Add unreleased part to the CHANGELOG.md
* Update Makefile to next version
* Git add all the modified files
* Git commit

### Manual
* Push the changes

# How to release OperatorHub bundle

OperatorHub has been split to two different community parts, one appearing for Kubernetes OLM installations and one for Openshift's integrated OperatorHub instance. We need to publish to both.
It happens by creating a PR to two different repositories:

https://github.com/redhat-openshift-ecosystem/community-operators-prod
https://github.com/k8s-operatorhub/community-operators

A script has been created which will create the necessary packages, branch and a commit. The prerequisite is that those two must be checked out to same level directory as cass-operator so that they
can be accessed with ``../community-operators/`` and ``../community-operators-prod``. Only users in the ``cass-operator-community/ci.yaml`` in those repository can submit updates. The commits
must be signed, thus ensure your git properties are correctly set.

The script is run by first checking out the correct tag in the ``cass-operator`` repository and then running:

``scripts/release-community-bundles.sh version`` 

where ``version`` is without ``v``-prefix, for example ``1.11.0``. You need to manually push the created branch ``cass-operator-$VERSION`` and make a PR, the script will not do that part.

# How to release Red Hat certified bundles

* Publish the container images (server-system-logger and cass-operator) from the portal
* If the system-logger image was updated, modify ``scripts/release-certified-bundles.sh`` with the new SHA256  

Checkout certified-operators (and if required - not at the moment, certified-operators-marketplace) to a directory at the same level as cass-operator. 

Copy the SHA256 of the published images from the portal and run the following script:

``scripts/release-certified-bundles.sh version <cass-operator-sha256> <system-logger-sha256>``, version without ``v`` prefix and sha256s with the ``sha256:`` prefix (the copy button on the portal puts it automatically). For example:

```
scripts/release-certified-bundles.sh 1.13.0 sha256:4f7262271839e23e20685762e2172ded0958d8235d49038814fd8cd3747b2690 sha256:43231f5e98cedad2620c71fb6db7e8e685cf825d2240f16aa615459dba241805
```

When sending the PR, the PR must have title structured: ``operator cass-operator (v$VERSION)``, such as ``operator cass-operator (v1.10.1)``. Also, the user must be added to the accepted user list from the Red Hat's portal (project settings).
