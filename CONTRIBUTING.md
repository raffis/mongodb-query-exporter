## Release process

### Controller release
1. Merge all pr's to master which need to be part of the new release
2. Create pr to master with these changes:
  1. Bump kustomization
  2. Create CHANGELOG.md entry with release and date
3. Merge pr
4. Push a tag following semantic versioning prefixed by 'v'. Do not create a github release, this is done automatically.
5. Create new branch and add the following changes:
  1. Bump chart version
  2. Bump charts app version
6. Create pr to master and merge

### Helm chart change only
1. Create branch with changes
2. Bump chart version
3. Create pr to master and merge
