# driftline

driftline synchronizes files from a source repository into a target repository using explicit source and target configuration. This context defines the product language for managed/template file sync.

## Language

**Source Repository**:
The repository that defines file identity, source paths, and file mode.
_Avoid_: Upstream package, template package.

**Target Repository**:
The repository where driftline places and synchronizes files from a Source Repository.
_Avoid_: Client repo, destination project.

**Source Config**:
The Source Repository's manifest of file groups, file identifiers, source paths, and file modes.
_Avoid_: Source schema, package manifest.

**Target manifest**:
The Target Repository's human-editable record of the Source Repository, source ref, and target paths for currently Managed files.
_Avoid_: Lock file, state file.

**Managed file**:
A Source Repository file that driftline keeps synchronized in the Target Repository.
_Avoid_: Installed file, owned file.

**Template file**:
A Source Repository file used only for initial placement; after placement it becomes target-owned.
_Avoid_: Optional managed file, one-shot managed file.

**File key**:
The stable `<group>.<file>` identity for a file declared by the Source Config and, for Managed files, referenced by the Target manifest.
_Avoid_: Path key, config path.

**Sync plan**:
The desired set of Target Repository changes derived from the current Source Config, Target manifest, source file bytes, and target file state.
_Avoid_: Migration, transaction log.

## Example Dialogue

Dev: The Source Config changed `github-workflow.ci` from a Template file to a Managed file.

Domain expert: Then the Sync plan should treat the existing target path as target-owned unless the Target manifest already records that File key.

Dev: If it is target-owned, driftline reports a conflict instead of overwriting it.

Domain expert: Right. The Target manifest is the only current record of Managed files; there is no lock file or historical ownership state.
