# How to make a release

* Make a `release` branch (such as 1.8.x)
* Run all int tests
* Update `CHANGELOG.md`
* Update `README.md` links/references to the right version in URLs like `https://.../k8ssandra/cass-operator/v1.8.0/...`
* Create a tag and watch that release process completes in the github actions
