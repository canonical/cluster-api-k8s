import datetime
from unittest import mock

import promote
import pytest
from packaging.version import Version
from promote import ReleaseType


@mock.patch("subprocess.run")
def test_get_tags(mock_run):
    mock_run.return_value = mock.Mock(
        stdout=" some_tag  \n  some_other_tag ", stderr=""
    )

    tags = promote.get_tags()
    assert tags == ["some_tag", "some_other_tag"]

    mock_run.assert_called_once_with(
        ["git", "tag", "-l", "--merged"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd=None,
        capture_output=True,
        text=True,
    )


@mock.patch("subprocess.run")
def test_get_tags_with_ref_and_clone_dir(mock_run):
    mock_run.return_value = mock.Mock(stdout="some_tag\nsome_other_tag", stderr="")

    tags = promote.get_tags(ref="remote/branch", clone_dir="clone_dir")
    assert tags == ["some_tag", "some_other_tag"]

    mock_run.assert_called_once_with(
        ["git", "tag", "-l", "--merged", "remote/branch"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone_dir",
        capture_output=True,
        text=True,
    )


@mock.patch("subprocess.run")
def test_get_tags_with_ref_no_tags(mock_run):
    mock_run.return_value = mock.Mock(stdout="", stderr="")

    tags = promote.get_tags(ref="origin/release-1.2")
    assert tags == []

    mock_run.assert_called_once_with(
        ["git", "tag", "-l", "--merged", "origin/release-1.2"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd=None,
        capture_output=True,
        text=True,
    )


@mock.patch("subprocess.run")
def test_fetch_remote(mock_run):
    promote.fetch_remote("remote", "clone-dir")

    mock_run.assert_called_once_with(
        ["git", "fetch", "remote"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone-dir",
        capture_output=True,
        text=True,
    )


@mock.patch("subprocess.run")
def test_get_tags_pointing_at_commit(mock_run):
    mock_run.return_value = mock.Mock(stdout="some_tag\nsome_other_tag", stderr="")

    tags = promote.get_tags_pointing_at_commit("commit-id", clone_dir="clone_dir")
    assert tags == ["some_tag", "some_other_tag"]

    mock_run.assert_called_once_with(
        ["git", "tag", "--points-at", "commit-id"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone_dir",
        capture_output=True,
        text=True,
    )


def test_get_tags_pointing_at_commit_invalid():
    with pytest.raises(ValueError):
        promote.get_tags_pointing_at_commit("", "clone-dir")


@mock.patch("subprocess.run")
def test_get_tag_timestamp(mock_run):
    timestamp = 1738339292
    mock_run.return_value = mock.Mock(stdout=f"{timestamp}", stderr="")

    ret_val = promote.get_tag_timestamp("test-tag", clone_dir="clone_dir")
    exp_timestamp = datetime.datetime.fromtimestamp(timestamp, datetime.UTC)
    assert exp_timestamp == ret_val

    mock_run.assert_called_once_with(
        ["git", "tag", "-l", "test-tag", r"--format='%(creatordate:unix)'"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone_dir",
        capture_output=True,
        text=True,
    )


@mock.patch.object(promote, "get_tag_timestamp")
def test_get_tag_age(mock_get_timestamp):
    tag_timestamp = datetime.datetime(2025, 3, 10, 0, 0, 0, 0)
    now = datetime.datetime(2025, 3, 11, 0, 0, 0, 0)
    exp_delta = datetime.timedelta(days=1)

    mock_get_timestamp.return_value = tag_timestamp

    with mock.patch.object(datetime, "datetime") as mock_datetime:
        mock_datetime.now.return_value = now
        return_value = promote.get_tag_age("test-tag", "clone-dir")

    assert exp_delta == return_value

    mock_get_timestamp.assert_called_once_with("test-tag", "clone-dir")


@mock.patch("subprocess.run")
@pytest.mark.parametrize(
    "ret_timestamp",
    [
        # Multiple timestamps retrieved.
        "1738339292\n1738339999",
        # No timestamps retrieved.
        "\n",
        # Invalid timestamp.
        "invalid-timestamp",
    ],
)
def test_get_tag_timestamp_invalid(mock_run, ret_timestamp):
    mock_run.return_value = mock.Mock(stdout=ret_timestamp, stderr="")
    with pytest.raises(ValueError):
        promote.get_tag_timestamp("test-tag")


def test_get_tag_timestamp_no_commit():
    with pytest.raises(ValueError):
        promote.get_tag_timestamp("")


@mock.patch("subprocess.run")
def test_get_branches(mock_run):
    mock_run.return_value = mock.Mock(
        stdout=" some_branch\n  some_other_branch", stderr=""
    )

    tags = promote.get_branches()
    assert tags == ["some_branch", "some_other_branch"]

    mock_run.assert_called_once_with(
        ["git", "branch", "-r"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd=None,
        capture_output=True,
        text=True,
    )


@mock.patch("subprocess.run")
def test_create_tag(mock_run):
    promote.create_tag("tag-name", "commit-id", "clone-dir")
    mock_run.assert_called_once_with(
        ["git", "tag", "tag-name", "commit-id"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone-dir",
        capture_output=True,
        text=True,
    )


def test_create_tag_invalid():
    with pytest.raises(ValueError):
        promote.create_tag("", "commit-id")

    with pytest.raises(ValueError):
        promote.create_tag("tag-name", "")


@mock.patch("subprocess.run")
def test_push_tag(mock_run):
    promote.push_tag("tag-name", "remote-name", "clone-dir")
    mock_run.assert_called_once_with(
        ["git", "push", "remote-name", "tag-name"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone-dir",
        capture_output=True,
        text=True,
    )


def test_push_tag_invalid():
    with pytest.raises(ValueError):
        promote.push_tag("")


@mock.patch("subprocess.run")
def test_get_branches_with_clone_dir(mock_run):
    mock_run.return_value = mock.Mock(
        stdout="some_branch\nsome_other_branch", stderr=""
    )

    tags = promote.get_branches(clone_dir="clone_dir")
    assert tags == ["some_branch", "some_other_branch"]

    mock_run.assert_called_once_with(
        ["git", "branch", "-r"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd="clone_dir",
        capture_output=True,
        text=True,
    )


@pytest.mark.parametrize(
    "all_branches, remote, expected_branches",
    [
        (
            [
                "origin/main",
                "origin/release-0.1",
                "origin/release-0.2",
                "origin/KU-10101",
                "origin/user/branch",
                "myrepo/main",
                "myrepo/release-1.1",
                "myrepo/dev-branch",
            ],
            "origin",
            ["origin/main", "origin/release-0.1", "origin/release-0.2"],
        ),
        (
            [
                "origin/main",
                "origin/release-0.1",
                "origin/release-0.2",
                "origin/KU-10101",
                "origin/user/branch",
                "myrepo/main",
                "myrepo/release-1.1",
                "myrepo/dev-branch",
            ],
            "myrepo",
            ["myrepo/main", "myrepo/release-1.1"],
        ),
    ],
)
@mock.patch.object(promote, "get_branches")
def test_get_managed_branches(
    mock_get_branches, all_branches, remote, expected_branches
):
    mock_get_branches.return_value = all_branches
    return_value = promote.get_managed_branches(clone_dir="clone_dir", remote=remote)
    assert expected_branches == return_value


@mock.patch("subprocess.run")
def test_get_commit_id(mock_run):
    mock_run.return_value = mock.Mock(stdout="fake-commit-id\n", stderr="")

    commit_id = promote.get_commit_id("fake-ref")
    assert "fake-commit-id" == commit_id

    mock_run.assert_called_once_with(
        ["git", "rev-list", "-n", "1", "fake-ref"],
        check=True,
        timeout=promote.EXEC_TIMEOUT,
        cwd=None,
        capture_output=True,
        text=True,
    )


def test_get_commit_id_no_ref():
    with pytest.raises(ValueError):
        promote.get_commit_id("")


@mock.patch("subprocess.run")
def test_get_commit_id_not_found(mock_run):
    mock_run.return_value = mock.Mock(stdout="", stderr="")
    with pytest.raises(LookupError):
        promote.get_commit_id("fake-ref")


@mock.patch.object(promote, "get_tags")
def test_get_versions(mock_get_tags):
    mock_get_tags.return_value = [
        "v1.1.0",
        "v1.0.3",
        "2.1.0-alpha.0",
        "some-invalid-tag",
    ]
    # Tags that do not follow the semver format are ignored.
    # Valid tags are returned in semantic order.
    exp_versions = [
        Version("2.1.0-a.0"),
        Version("1.1.0"),
        Version("1.0.3"),
    ]

    versions = promote.get_versions()
    assert exp_versions == versions


@mock.patch.object(promote, "get_tags")
def test_get_latest_version(mock_get_tags):
    mock_get_tags.return_value = ["1.1.0", "1.0.3", "2.1.0-alpha.0", "some-invalid-tag"]
    latest_version = promote.get_latest_version()
    assert Version("2.1.0-a0") == latest_version


@mock.patch.object(promote, "get_tags")
def test_get_latest_version_missing(mock_get_tags):
    mock_get_tags.return_value = []
    with pytest.raises(ValueError):
        promote.get_latest_version()


@pytest.mark.parametrize(
    "tags, release_type, exp_ret",
    [
        (["1.0.0", "1.1.0-alpha.1"], ReleaseType.MINOR, Version("1.1.0-a.2")),
        (["1.1.0-alpha.1", "1.1.0-rc.0"], ReleaseType.MINOR, Version("1.1.0-rc.1")),
        (["1.0.0", "1.3.0", "1.4.1"], ReleaseType.MINOR, Version("1.5.0-a.0")),
        (["1.0.0", "1.3.0", "1.4.1"], ReleaseType.MICRO, Version("1.4.2-a.0")),
        (["1.0.0", "1.3.0", "1.4.1"], ReleaseType.MAJOR, Version("2.0.0-a.0")),
    ],
)
@mock.patch.object(promote, "get_tags")
def test_get_next_prerelease_version(mock_get_tags, tags, release_type, exp_ret):
    mock_get_tags.return_value = tags

    ret_val = promote.get_next_prerelease_version(release_type, "branch", "clone-dir")
    assert exp_ret == ret_val


@mock.patch.object(promote, "get_tags")
def test_get_next_prerelease_version_invalid(
    mock_get_tags,
):
    mock_get_tags.return_value = ["1.0.0", "1.1.0"]

    with pytest.raises(ValueError):
        promote.get_next_prerelease_version(ReleaseType.UNKNOWN, "branch", "clone-dir")


@pytest.mark.parametrize(
    "tags, release",
    [
        # Stable releases will not be promoted.
        (["1.0.0", "1.0.1"], Version("1.2.0")),
        # Unknown prerelease.
        (["1.0.0", "1.0.1"], mock.Mock(pre=("myBeta", 1))),
    ],
)
@mock.patch.object(promote, "get_tags")
def test_get_promoted_release_invalid(mock_get_tags, tags, release):
    mock_get_tags.return_value = tags
    all_versions = promote.get_versions()
    with pytest.raises(ValueError):
        promote.get_promoted_release(release, all_versions=all_versions)


@pytest.mark.parametrize(
    "tags, release, exp_promoted",
    [
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.2-a2"),
            Version("1.0.2-b4"),
        ),
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.1-b.1"),
            Version("1.0.1-rc.0"),
        ),
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.3-b.5"),
            Version("1.0.3-rc.0"),
        ),
        (
            ["v1.0.0", "v1.0.2-alpha.2", "v1.0.2-beta.3", "v1.0.1-beta.1"],
            Version("1.0.3-rc.4"),
            Version("1.0.3"),
        ),
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.0-rc.2"),
            Version("1.0.1"),
        ),
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.4-rc.5"),
            Version("1.0.4"),
        ),
        (
            ["1.0.0", "1.0.2-alpha.2", "1.0.2-beta.3", "1.0.1-beta.1"],
            Version("1.0.2-rc.3"),
            Version("1.0.2"),
        ),
    ],
)
@mock.patch.object(promote, "get_tags")
def test_get_promoted_release(mock_get_tags, tags, release, exp_promoted):
    mock_get_tags.return_value = tags
    all_versions = promote.get_versions()

    ret_val = promote.get_promoted_release(release, all_versions=all_versions)
    assert exp_promoted == ret_val


@pytest.mark.parametrize(
    "version, exp_tag",
    [
        (Version("1.0.0"), "v1.0.0"),
        (Version("1.0.0-a.1"), "v1.0.0-alpha.1"),
        (Version("1.0.0-b.5"), "v1.0.0-beta.5"),
        (Version("1.0.0-rc.0"), "v1.0.0-rc.0"),
    ],
)
def test_version_to_tag(version, exp_tag):
    ret_val = promote.version_to_tag(version)
    assert exp_tag == ret_val


def test_version_to_tag_invalid():
    with pytest.raises(ValueError):
        promote.version_to_tag(mock.Mock(pre=("myBeta", 1)))


@pytest.mark.parametrize("dry_run", (True, False))
@mock.patch.object(promote, "fetch_remote")
@mock.patch.object(promote, "push_tag")
@mock.patch.object(promote, "create_tag")
@mock.patch.object(promote, "get_commit_id")
@mock.patch.object(promote, "get_branches")
@mock.patch.object(promote, "get_tags")
def test_create_new_prereleases(
    mock_get_tags,
    mock_get_branches,
    mock_get_commit_id,
    mock_create_tag,
    mock_push_tag,
    mock_fetch_remote,
    dry_run,
):
    branches = [
        "origin/main",
        "origin/release-0.4",
        "origin/release-0.5",
        "origin/release-0.6",
        "origin/release-0.7",
        "myrepo/main",
        "myrepo/dev",
    ]

    tags = {
        "origin/main": ["v1.0.0"],
        "origin/release-0.4": ["v0.4.0"],
        "origin/release-0.5": ["v0.5.1"],
        "origin/release-0.6": [],
        "origin/release-0.7": ["v0.7.0", "v0.7.1-beta.0"],
    }

    # We'll use some meaningful names instead of commit id hashes,
    # making the unit tests more readable.
    commit_for_ref = {
        "origin/main": "untagged-main",
        "origin/release-0.4": "untagged-0.4",
        "origin/release-0.5": "v0.5.1",
        "origin/release-0.6": "untagged-0.6",
        "origin/release-0.7": "untagged-0.7",
        "v1.0.0": "v1.0.0",
        "v0.4.0": "v0.4.0",
        "v0.5.1": "v0.5.1",
        "v0.7.0": "v0.7.0",
        "v0.7.1-beta.0": "v0.7.1-beta.0",
    }

    mock_get_branches.return_value = branches
    mock_get_tags.side_effect = lambda ref, clone_dir=None: tags[ref]
    mock_get_commit_id.side_effect = lambda ref, clone_dir=None: commit_for_ref[ref]

    promote.create_new_prereleases(
        dry_run=dry_run, clone_dir="clone-dir", remote="origin"
    )

    mock_fetch_remote.assert_called_once_with(remote="origin", clone_dir="clone-dir")
    if dry_run:
        mock_create_tag.assert_not_called()
        mock_push_tag.assert_not_called()
    else:
        mock_create_tag.assert_has_calls(
            [
                mock.call(
                    name="v1.1.0-alpha.0",
                    commit_id="untagged-main",
                    clone_dir="clone-dir",
                ),
                mock.call(
                    name="v0.4.1-alpha.0",
                    commit_id="untagged-0.4",
                    clone_dir="clone-dir",
                ),
                mock.call(
                    name="v0.7.1-beta.1",
                    commit_id="untagged-0.7",
                    clone_dir="clone-dir",
                ),
            ]
        )
        mock_push_tag.assert_has_calls(
            [
                mock.call(
                    name="v1.1.0-alpha.0", remote="origin", clone_dir="clone-dir"
                ),
                mock.call(
                    name="v0.4.1-alpha.0", remote="origin", clone_dir="clone-dir"
                ),
                mock.call(name="v0.7.1-beta.1", remote="origin", clone_dir="clone-dir"),
            ]
        )


@pytest.mark.parametrize("dry_run", (True, False))
@mock.patch.object(promote, "push_tag")
@mock.patch.object(promote, "create_tag")
@mock.patch.object(promote, "get_commit_id")
def test_promote_release(mock_get_commit_id, mock_create_tag, mock_push_tag, dry_run):
    mock_get_commit_id.return_value = "commit-id"

    promote.promote_release(
        Version("1.0.0-alpha.0"),
        Version("1.0.0-beta.0"),
        dry_run=dry_run,
        remote="remote",
        clone_dir="clone-dir",
    )

    if dry_run:
        mock_create_tag.assert_not_called()
        mock_push_tag.assert_not_called()
    else:
        mock_create_tag.assert_called_once_with(
            name="v1.0.0-beta.0", commit_id="commit-id", clone_dir="clone-dir"
        )
        mock_push_tag.assert_called_once_with(
            name="v1.0.0-beta.0", remote="remote", clone_dir="clone-dir"
        )


@pytest.mark.parametrize(
    "dry_run, promote_immediately",
    [
        (True, False),
        (True, True),
        (False, True),
        (False, False),
    ],
)
@mock.patch.object(promote, "fetch_remote")
@mock.patch.object(promote, "push_tag")
@mock.patch.object(promote, "create_tag")
@mock.patch.object(promote, "get_commit_id")
@mock.patch.object(promote, "get_tag_age")
@mock.patch.object(promote, "get_tags")
def test_promote_releases(
    mock_get_tags,
    mock_get_tag_age,
    mock_get_commit_id,
    mock_create_tag,
    mock_push_tag,
    mock_fetch_remote,
    dry_run,
    promote_immediately,
):
    # tag -> tag age
    tags = {
        # Stable release, not promoted.
        "v0.3.1": datetime.timedelta(days=1),
        # Newer release available, not promoted.
        "v0.3.1-alpha.0": datetime.timedelta(days=2),
        # Not enough time passed, not promoted.
        "v0.4.0-beta.0": datetime.timedelta(days=2),
        # Newer semantic release available, not promoted.
        "v0.4.0-alpha.0": datetime.timedelta(days=1),
        # No stable release for v0.5, manual release required.
        "v0.5.0-rc.1": datetime.timedelta(days=20),
        # Promotion allowed, v0.6.0 available
        "v0.6.1-rc.3": datetime.timedelta(days=20),
        # Stable release, not promoted.
        "v0.6.0": datetime.timedelta(days=20),
        # Promotion allowed.
        "v0.7.0-alpha.3": datetime.timedelta(days=2),
        # Promotion allowed.
        "v0.8.0-beta.3": datetime.timedelta(days=4),
    }

    expected_promotions = [
        # from -> to
        ("v0.6.1-rc.3", "v0.6.1"),
        ("v0.7.0-alpha.3", "v0.7.0-beta.0"),
        ("v0.8.0-beta.3", "v0.8.0-rc.0"),
    ]
    if promote_immediately:
        expected_promotions += [
            ("v0.4.0-beta.0", "v0.4.0-rc.0"),
        ]

    mock_get_tags.return_value = tags.keys()
    mock_get_commit_id.side_effect = lambda tag, clone_dir="": f"{tag}-commit"
    mock_get_tag_age.side_effect = lambda tag: tags[tag]

    promote.promote_releases(
        dry_run=dry_run,
        promote_immediately=promote_immediately,
        clone_dir="clone-dir",
        remote="remote",
    )

    mock_fetch_remote.assert_called_once_with(remote="remote", clone_dir="clone-dir")
    if dry_run:
        mock_create_tag.assert_not_called()
        mock_push_tag.assert_not_called()
    else:
        mock_create_tag.assert_has_calls(
            [
                mock.call(
                    name=to_tag, commit_id=f"{from_tag}-commit", clone_dir="clone-dir"
                )
                for from_tag, to_tag in expected_promotions
            ],
            any_order=True,
        )
        mock_push_tag.assert_has_calls(
            [
                mock.call(name=to_tag, remote="remote", clone_dir="clone-dir")
                for from_tag, to_tag in expected_promotions
            ],
            any_order=True,
        )


@pytest.mark.parametrize("return_value", ["string output", ["array", "output"]])
@mock.patch.object(promote, "create_new_prereleases")
def test_main(mock_create_new_releases, return_value):
    mock_create_new_releases.return_value = return_value
    args = [
        "promote.py",
        "create_new_prereleases",
        "--clone-dir",
        "clone_dir",
        "--dry-run",
        "--remote",
        "fake-remote",
    ]
    with mock.patch("sys.argv", args):
        promote.main()

    mock_create_new_releases.assert_called_once_with(
        clone_dir="clone_dir", dry_run=True, remote="fake-remote"
    )
