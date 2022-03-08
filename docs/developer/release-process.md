# How to make a release

After ensuring the release is done and integration tests work:

Run the following sequence, doing the small manual steps when required (until automated). There is no need to make a release branch as we want to maintain documentation in the master also.
Ensure the post-release and pre-release scripts are correct if you've made any changes to the structure of the project since the last release.

The steps required (assuming releasing v1.10.0 and previous one was v1.9.0), doing from master (replace git commands with correct target remote names):

```
scripts/pre-release-process.sh v1.10.0 v1.9.0
git commit -m 'Release v1.10.0'
git tag v1.10.0
git push upstream master
git push upstream v1.10.0
```

```
scripts/post-release-process.sh
# Do post-release manual work
git commit -m 'Post-release v1.10.0'
git push upstream master
```

The github action will create docker images and bundles.

### pre-release

#### scripts/pre-release-process.sh
* Modify CHANGELOG automatically to match the tag (for unreleased)
* Modify README to include proper installation refs (keep them also?)
* Modify config/manager/kustomization.yaml to have proper newTag for cass-operator
* Modify config/manager/image_config.yaml to have proper version for server-system-logger

#### GHA / manual
* Commit changes

### release-github-action

* Copy the unreleased notes to Github releases
* Add Github packages upload for docker images

### post-release

#### scripts/post-release-process.sh
* Return config/manager/kustomization.yaml to :latest
* Return config/manager/image_config.yaml to :latest

#### GHA / manual
* Commit changes

#### Manual work (for now)
* Add also new ## unreleased after the tagging (post-release-process.sh)
* Modify Makefile for the next VERSION in line

# How to release OperatorHub bundle

OperatorHub has been split to two different community parts, one appearing for Kubernetes OLM installations and one for Openshift's integrated OperatorHub instance. We need to publish to both.
It happens by creating a PR to two different repositories:

https://github.com/redhat-openshift-ecosystem/community-operators-prod
https://github.com/k8s-operatorhub/community-operators

A script has been created which will create the necessary packages, branch and a commit. The prerequisite is that those two must be checked out to same level directory as cass-operator so that they
can be accessed with ``../community-operators/`` and ``../community-operators-prod``. Only users in the ``cass-operator-community/ci.yaml`` in those repository can submit updates. The commits
must be signed, thus ensure your git properties are correctly set.

The script is run by first checking out the correct tag in the ``cass-operator`` repository and then running:

``scripts/release-community-olm.sh version`` 

where ``version`` is without ``v``-prefix, for example ``1.11.0``. You need to manually push the created branch ``cass-operator-$VERSION`` and make a PR, the script will not do that part.

# How to release Red Hat certified bundles

* Upload images to the Red Hat using the connect.redhat.com portal
* Select container images and "push manually" for instructions. Upload cass-operator and system-logger if you wish to update that one also (if there are no changes, then no need to)
* If the images are built using ubi8-micro, you need to make a support ticket to ask them for a waiver since their validation process is broken. This happens after the upload is done and the
  only failure is rpm_list
* Include oisp, project_id, sha256 of the image to the ticket
* Proceed only after you've received OK from them (the state on the portal changes to "Passed")
* Publish the container image from the portal
* If the system-logger image was updated, modify ``scripts/release-certified-bundles.sh`` with the new SHA256  

Checkout certified-operators (and if required, certified-operators-marketplace) to a directory at the same level as cass-operator. 

Copy the SHA256 of the published image and run the following script:

``scripts/release-certified-bundles.sh version sha256:<sha256>``, version without ``v`` prefix and sha256 with the ``sha256:`` prefix (the copy button on the portal puts it automatically). For example:

```
scripts/release-certified-bundles.sh 1.10.1 sha256:ae709b680dde2aa43c92d6b331af0554c9af92aa9fad673454892ca2b40bd3f7
```

When sending the PR, the PR must have title structured: ``operator cass-operator (v$VERSION)``, such as ``operator cass-operator (v1.10.1)``. Also, the user must be added to the accepted user list from the Red Hat's portal (project settings).
