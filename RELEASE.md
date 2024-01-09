# Release Process

We take inspiration from the controller-runtime release process:
https://github.com/kubernetes-sigs/controller-runtime/blob/main/RELEASE.md

1. Create a new release branch from main: `git checkout -b release-<MAJOR.MINOR>`
2. Push the new branch to the remote repository: ` git push --set-upstream origin release-<MAJOR.MINOR>`
3. Fetch all tags from the remote: `git fetch --all --tags`
4. Generate the release notes from the previous tag: `go run ./hack/notes --from vMAJOR.MINOR.PATCH`
5. Create a new release in GitHub from the release branch, pasting the generated release notes
