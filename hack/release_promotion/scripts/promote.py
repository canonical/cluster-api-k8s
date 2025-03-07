#!/usr/bin/env python3

# See the readme file for an explanation.

import argparse
import datetime
import enum
import logging
import os
import re
import subprocess
from typing import List, Optional

from packaging.version import InvalidVersion, Version

K8S_TAGS_URL = "https://api.github.com/repos/kubernetes/kubernetes/tags"
EXEC_TIMEOUT = 60

LOG = logging.getLogger(__name__)

# We'll automatically tag branches that match the following regex.
MANAGED_BRANCH_RE = re.compile(r"main|release-.*")

# How long to wait before promoting prereleases, based on risk level.
PRERELEASE_PROMOTION_INTERVAL = {
    "a": datetime.timedelta(days=1),
    "b": datetime.timedelta(days=3),
    "rc": datetime.timedelta(days=5),
}


class ReleaseType(enum.Enum):
    MAJOR = "major"
    MINOR = "minor"
    MICRO = "micro"
    UNKNOWN = "unknown"


def _exec(cmd: List[str], check=True, timeout=EXEC_TIMEOUT, cwd=None):
    """Run the specified command and return the stdout/stderr output as a tuple."""
    LOG.debug("Executing: %s, cwd: %s.", cmd, cwd)
    proc = subprocess.run(
        cmd, check=check, timeout=timeout, cwd=cwd, capture_output=True, text=True
    )
    return proc.stdout, proc.stderr


def get_tags(ref: str = "", clone_dir: Optional[str] = None) -> List[str]:
    """Returns all git tags.

    ref: can be a branch (e.g. $ref/$branch), in which case only the tags
         from that branch will be retrieved.
    clone_dir: the git clone location.
    """
    cmd = ["git", "tag", "-l", "--merged"]
    if ref:
        cmd.append(ref)
    stdout, stderr = _exec(cmd, cwd=clone_dir)
    return [tag.strip(" ") for tag in stdout.split("\n") if tag]


def get_tags_pointing_at_commit(
    commit_id: str, clone_dir: Optional[str] = None
) -> List[str]:
    """Returns all git tags that point to a given commit.

    commit_id: the commit id associated with the tags.
    clone_dir: the git clone location.
    """
    if not commit_id:
        raise ValueError("No commit specified.")

    cmd = ["git", "tag", "--points-at", commit_id]
    stdout, stderr = _exec(cmd, cwd=clone_dir)
    return [tag.strip(" ") for tag in stdout.split("\n") if tag]


def get_tag_timestamp(tag: str, clone_dir: Optional[str] = None) -> datetime.datetime:
    if not tag:
        raise ValueError("No tag specified.")

    cmd = ["git", "tag", "-l", tag, r"--format='%(creatordate:unix)'"]
    stdout, stderr = _exec(cmd, cwd=clone_dir)
    timestamps = [tag.strip(" ") for tag in stdout.split("\n") if tag]
    if len(timestamps) > 1:
        raise ValueError(f"Multiple timestamps detected for tag {tag}: {timestamps}.")
    if not timestamps:
        raise ValueError(f"Couldn't determine timestamp for tag {tag}.")

    timestamp = timestamps[0]
    try:
        return datetime.datetime.fromtimestamp(int(timestamp), datetime.UTC)
    except Exception as ex:
        raise ValueError(
            f"Couldn't parse tag timestamp: {tag} {timestamp}, exception: {ex}"
        )


def get_tag_age(tag: str, clone_dir: Optional[str] = None) -> datetime.timedelta:
    timestamp = get_tag_timestamp(tag, clone_dir)
    return datetime.datetime.now() - timestamp


def get_branches(clone_dir: Optional[str] = None) -> List[str]:
    """Returns all branches.

    clone_dir: the git clone location.
    """
    stdout, stderr = _exec(["git", "branch", "-r"], cwd=clone_dir)
    return [branch.strip(" ") for branch in stdout.split("\n") if branch]


def create_tag(name: str, commit_id: str, clone_dir: Optional[str] = None):
    if not name:
        raise ValueError("Missing tag name.")

    if not commit_id:
        raise ValueError("Missing commit id.")

    cmd = ["git", "tag", name, commit_id]
    _exec(cmd, cwd=clone_dir)


def push_tag(name: str, remote: str = "origin", clone_dir: Optional[str] = None):
    if not name:
        raise ValueError("Missing tag name.")

    cmd = ["git", "push", remote, name]
    _exec(cmd, cwd=clone_dir)


def get_managed_branches(
    clone_dir: Optional[str] = None, remote: str = "origin"
) -> List[str]:
    branches = get_branches(clone_dir=clone_dir)

    managed_branches = []
    for branch in branches:
        found_remote, branch_name = branch.split("/", 1)
        if found_remote == remote and MANAGED_BRANCH_RE.match(branch_name):
            managed_branches.append(branch)
    return managed_branches


def get_commit_id(ref: str, clone_dir: Optional[str] = None) -> str:
    """Returns the latest commit id for a given ref (e.g. tag, branch, etc)"""
    if not ref:
        raise ValueError("No ref specified.")

    stdout, stderr = _exec(["git", "rev-list", "-n", "1", ref], cwd=clone_dir)
    commit_id = stdout.strip(" \n")
    if not commit_id:
        raise LookupError(f"No commit found for ref: {ref}")
    return commit_id


def get_versions(ref: str = "", clone_dir: Optional[str] = None) -> List[Version]:
    """Returns all semantic versions based on the git tags.

    ref: can be a branch (e.g. $ref/$branch), in which case only the tags
         from that branch will be retrieved.
    """
    tags = get_tags(ref=ref, clone_dir=clone_dir)
    versions = []
    for tag in tags:
        try:
            ver = Version(tag)
        except InvalidVersion:
            LOG.warning("Not a valid semantic version, ignoring tag: %s", tag)
            continue
        versions.append(ver)

    # Return neweset first.
    versions.sort(reverse=True)
    return versions


def get_latest_version(ref: str = "", clone_dir: Optional[str] = None) -> Version:
    versions = get_versions(ref=ref, clone_dir=clone_dir)
    if not versions:
        raise ValueError("No versions detected.")
    return versions[0]


def get_next_prerelease_version(
    release_type: ReleaseType = ReleaseType.MINOR,
    ref: str = "",
    clone_dir: Optional[str] = None,
) -> Version:
    """Get the next prerelease version for the specified branch.

    ref: a branch such as origin/main or custom-remote/release-0.3.
    clone_dir: the git clone location.
    release_type: determines which part of the version will be bumped if the
                  latest release is a stable release.
    """
    lv = get_latest_version(ref=ref, clone_dir=clone_dir)
    if lv.pre:
        pre = lv.pre[0], lv.pre[1] + 1
        version_str = f"{lv.major}.{lv.minor}.{lv.micro}-{pre[0]}.{pre[1]}"
    else:
        # The latest version is a stable release.
        if release_type == ReleaseType.MAJOR:
            version_str = f"{lv.major + 1}.0.0-alpha.0"
        elif release_type == ReleaseType.MINOR:
            version_str = f"{lv.major}.{lv.minor + 1}.0-alpha.0"
        elif release_type == ReleaseType.MICRO:
            version_str = f"{lv.major}.{lv.minor}.{lv.micro + 1}-alpha.0"
        else:
            raise ValueError(f"Unknown release type: {release_type}")
    return Version(version_str)


def get_promoted_release(release: Version, all_versions: List[Version]) -> Version:
    if not release.pre:
        raise ValueError(
            f"Not a prerelease: {release}. Stable releases do not get promoted."
        )

    next_level_map = {"a": "b", "b": "rc", "rc": "stable"}
    next_level = next_level_map.get(release.pre[0])
    if not next_level:
        raise ValueError(f"Unable to determine next release level for {release}.")

    for existing in all_versions:
        if (
            existing.major == release.major
            and existing.minor == release.minor
            and existing.micro == release.micro
        ):
            if next_level == "stable" and existing.is_prerelease:
                continue
            if next_level != "stable" and (
                not existing.pre or existing.pre[0] != next_level
            ):
                continue
            if existing.pre:
                pre = existing.pre[0], existing.pre[1] + 1
                version_str = f"{existing.major}.{existing.minor}.{existing.micro}-{pre[0]}.{pre[1]}"
            else:
                version_str = f"{existing.major}.{existing.minor}.{existing.micro + 1}"
            return Version(version_str)

    # Couldn't find a matching release, create a new one.
    if next_level == "stable":
        version_str = f"{release.major}.{release.minor}.{release.micro}"
    else:
        pre = next_level, 0
        version_str = (
            f"{release.major}.{release.minor}.{release.micro}-{pre[0]}.{pre[1]}"
        )
    return Version(version_str)


def version_to_tag(version: Version) -> str:
    prerelease_map = {
        "a": "alpha",
        "b": "beta",
        "rc": "rc",
    }

    tag = f"v{version.major}.{version.minor}.{version.micro}"
    if version.pre:
        prerelease_name = prerelease_map.get(version.pre[0])
        if not prerelease_name:
            raise ValueError(
                f"Unknown prerelease name: {version.pre[0]}, "
                f"supported: {prerelease_map.keys()}"
            )
        tag += f"-{prerelease_name}.{version.pre[1]}"
    return tag


def create_new_prereleases(
    dry_run: bool = False, clone_dir: Optional[str] = None, remote: str = "origin"
):
    managed_branches = get_managed_branches(clone_dir=clone_dir, remote=remote)
    for branch in managed_branches:
        tags = get_tags(ref=branch, clone_dir=clone_dir)
        tip_commit = get_commit_id(ref=branch, clone_dir=clone_dir)

        if tags:
            latest_tag = tags[0]
            latest_tagged_commit = get_commit_id(ref=latest_tag, clone_dir=clone_dir)
        else:
            latest_tag = None
            latest_tagged_commit = None

        if latest_tagged_commit == tip_commit:
            LOG.info(
                f"No new commits on branch: {branch}. "
                f"Latest tag: {latest_tag}, latest commit: {latest_tagged_commit}."
            )
            continue

        # Bump the minor version on main and the micro version on stable branches.
        # For example, the minor version on release-1.1 should always stay the same.
        if branch.split("/")[1] == "main":
            next_prerelease_type = ReleaseType.MINOR
        else:
            next_prerelease_type = ReleaseType.MICRO

        try:
            next_prerelease_version = get_next_prerelease_version(
                release_type=next_prerelease_type, ref=branch, clone_dir=clone_dir
            )
            next_prerelease_tag = version_to_tag(next_prerelease_version)
        except ValueError:
            LOG.warning(
                f"Couldn't determine next prerelease for branch: {branch}. "
                "Make sure that at least one tag exists on that branch."
            )
            continue

        LOG.info(
            f"Creating tag: {next_prerelease_tag}, commit id: {tip_commit}, "
            f"branch: {branch}, dry run: {dry_run}."
        )
        if not dry_run:
            create_tag(
                name=next_prerelease_tag, commit_id=tip_commit, clone_dir=clone_dir
            )
            push_tag(name=next_prerelease_tag, remote=remote, clone_dir=clone_dir)


def newer_release_available_same_minor(
    version: Version, all_versions: List[Version]
) -> bool:
    for other_version in all_versions:
        if (
            version.major == other_version.major
            and version.minor == other_version.minor
            and version < other_version
        ):
            return True
    return False


def stable_release_available_same_minor(
    major: int, minor: int, all_versions: List[Version]
) -> bool:
    for version in all_versions:
        if version.pre:
            # Not a stable release
            continue
        if major == version.major and minor == version.minor:
            return True
    return False


def promote_release(
    from_version: Version,
    to_version: Version,
    dry_run: bool,
    clone_dir: Optional[str],
    remote: str,
):
    from_tag = version_to_tag(from_version)
    to_tag = version_to_tag(to_version)
    commit_id = get_commit_id(from_tag, clone_dir=clone_dir)

    LOG.info(f"Promoting release {from_tag} to {to_tag}, commit id: {commit_id}.")
    if not dry_run:
        create_tag(name=to_tag, commit_id=commit_id, clone_dir=clone_dir)
        push_tag(name=to_tag, remote=remote, clone_dir=clone_dir)


def promote_releases(
    dry_run: bool = False,
    promote_immediately=False,
    clone_dir: Optional[str] = None,
    remote: str = "origin",
):
    versions = get_versions(clone_dir=clone_dir)
    for version in versions:
        if not version.pre:
            LOG.debug(f"Not a prerelease, skipping: {version}.")
            continue

        if newer_release_available_same_minor(version, versions):
            LOG.debug(
                f"Newer {version.major}.{version.minor} release available, "
                f"skipping: {version}."
            )
            continue

        promoted_release = get_promoted_release(version, all_versions=versions)
        if not promoted_release.pre:
            # We're promoting to stable.
            minor_released = stable_release_available_same_minor(
                promoted_release.major, promoted_release.minor, versions
            )
            if not minor_released:
                LOG.warning(
                    "Refusing to automatically promote %s to %s (stable). "
                    "Prepare a branch and create the first stable release manually.",
                    version_to_tag(version),
                    version_to_tag(promoted_release),
                )
                continue

        promotion_interval = PRERELEASE_PROMOTION_INTERVAL.get(version.pre[0])
        if not promotion_interval:
            LOG.warning(
                f"Unable to determine promotion interval for prerelease {version}."
            )
            continue

        tag = version_to_tag(version)
        try:
            tag_age = get_tag_age(tag)
        except Exception as ex:
            LOG.warning(f"Unable to determine tag age: {tag}, exception: {ex}")
            continue

        if tag_age < promotion_interval:
            if promote_immediately:
                LOG.info(
                    f"The promotion interval hasn't elapsed yet for {tag}, "
                    "however --promote-immediately was passed. "
                    f"Age: {tag_age}, promotion interval: {promotion_interval}."
                )
            else:
                LOG.debug(
                    f"The promotion interval hasn't elapsed yet for {tag}."
                    f"Age: {tag_age}, promotion interval: {promotion_interval}."
                )
                continue

        promote_release(
            version,
            promoted_release,
            dry_run=dry_run,
            clone_dir=clone_dir,
            remote=remote,
        )


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--debug",
        dest="debug",
        help="Enable debug logging.",
        action="store_true",
    )

    subparsers = parser.add_subparsers(dest="subparser", required=True)

    def add_clone_dir_arg(cmd):
        cmd.add_argument(
            "--clone-dir",
            dest="clone_dir",
            help="The git clone directory.",
            default=os.getcwd(),
        )

    def add_dry_run_arg(cmd):
        cmd.add_argument(
            "--dry-run",
            dest="dry_run",
            help="Dry run mode, do not submit any tags.",
            action="store_true",
        )

    def add_remote_arg(cmd):
        cmd.add_argument(
            "--remote", dest="remote", help="Git remote.", default="origin"
        )

    def add_promote_immediately_arg(cmd):
        cmd.add_argument(
            "--promote-immediately",
            dest="promote_immediately",
            help="Promote prereleases without waiting for the standard interval.",
            action="store_true",
        )

    cmd = subparsers.add_parser("create_new_prereleases")
    add_clone_dir_arg(cmd)
    add_dry_run_arg(cmd)
    add_remote_arg(cmd)

    cmd = subparsers.add_parser("promote_releases")
    add_clone_dir_arg(cmd)
    add_dry_run_arg(cmd)
    add_remote_arg(cmd)
    add_promote_immediately_arg(cmd)

    kwargs = vars(parser.parse_args())

    debug = kwargs.pop("debug", False)
    log_level = logging.DEBUG if debug else logging.INFO
    logging.basicConfig(format="%(asctime)s %(message)s", level=log_level)

    f = globals()[kwargs.pop("subparser")]
    out = f(**kwargs)
    if isinstance(out, (list, tuple)):
        for item in out:
            print(item)
    else:
        print(out or "")


if __name__ == "__main__":
    main()
