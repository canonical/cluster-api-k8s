About
=====

This script can be used to automatically create prerelease tags and
promote them after a predetermined interval. The script is meant to be
executed as part of a nightly Github action.

Assumptions:

* the tags use semantic versioning:
    * ``v{major}.{minor}.{patch}-alpha.{pre}`` - alpha (edge) prerelease
    * ``v{major}.{minor}.{patch}-beta.{pre}`` - beta prerelease
    * ``v{major}.{minor}.{patch}-rc.{pre}`` - release candidate
    * ``v{major}.{minor}.{patch}`` - stable release
* there is one stable branch for each minor version
    * ``release-{major}.{minor}``
    * the branches and the first stable ``v{major}.{minor}.0`` tags are
      created manually
* unknown branches or tags are ignored

Creating prereleases
--------------------

The script checks for untagged commits against the main branch as well as
the ``release-{major}.{minor}`` branches.

If there are untagged commits, a new prerelease tag is created and submitted.

The new prerelease tag is determined based on the last tag associated with that
branch:

* if the last tag in semantic order is also a prerelease, we're increasing
  the prerelease number
    * v1.0.0-beta.0 -> v1.0.0-beta.1
* if the last tag in semantic order is a stable release:
    * for the main branch, we bump the minor version:
        * v1.0.0 -> v1.1.0-alpha.0
    * for stable branches, we bump the patch version:
        * v1.0.0 -> v1.0.1-alpha.0 

Promoting prereleases
---------------------

The script identifies all prerelease tags and promotes them (e.g. beta -> rc)
if:

1. There is no newer release with the same minor version.

* the newer release supersedes this tag and may fix critical regressions
  introduced by this tag
* this also avoids promoting the same tag twice

Example:

```
tags: v1.1.0-alpha.0, v1.1.0-alpha.1, v1.2.0-beta.1,v1.2.1-beta.0
promoted tags:
    * v1.1.0-alpha.1 -> v1.1.0-beta.0
    * v1.2.1-beta.0 -> v1.2.1-rc.0
```

2. Promoting to stable doesn't introduce a new minor version.

Release candidates from the main branch will not be promoted automatically.
The new branch and stable release tag must be created manually.

Release candidates from stable branches can be promoted automatically,
bumping the patch version.

3. A predetermined amount of time passed since the tag was created.

Promotion intervals:

* alpha: 1 day
* beta: 3 days
* release candidate: 5 days

For testing purposes, the promotion interval can be ignored by passing
``--promote-immediately``.
