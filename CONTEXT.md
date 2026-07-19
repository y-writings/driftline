# driftline

<!-- markdownlint-disable MD013 -->

driftline synchronizes files from a Source Repository into a Target Repository using an explicit Contract and Sync manifest. This context defines the product language for Managed/Template file sync and Contract Gitignore sections.

## Language

**Source Repository**:
The repository that defines file identity, source paths, and file mode.
_Avoid_: Upstream package, template package.

**Target Repository**:
The repository where driftline places and synchronizes files from a Source Repository.
_Avoid_: Client repo, destination project.

**Contract**:
The Source Repository's ref-scoped declaration of file groups, stable file identifiers, source paths, file modes, and raw entries for its partial region in the Target Repository's root `.gitignore`.
_Avoid_: Compatibility contract, package manifest, export receipt.

**Sync manifest**:
The Target Repository's human-editable, driftline-updatable record of one Source Repository/ref and target paths for currently Managed files. It does not record Gitignore section state.
_Avoid_: Lock file, state file, import receipt, bidirectional sync configuration.

**Managed file**:
A Source Repository file that driftline keeps synchronized in the Target Repository.
_Avoid_: Installed file, owned file.

**Template file**:
A Source Repository file used only for initial placement; after placement it becomes target-owned.
_Avoid_: Optional managed file, one-shot managed file.

**Gitignore section**:
The source-owned marker-delimited region in the Target Repository's root `.gitignore` that driftline reconciles from Contract `[gitignore].entries` while preserving target-owned bytes outside the markers.
_Avoid_: Managed `.gitignore`, appended ignore list, generated `.gitignore` file.

**File key**:
The stable `<group>.<file>` identity for a file declared by the Contract and, for Managed files, referenced by the Sync manifest.
_Avoid_: Path key, config path.

**Sync plan**:
The desired set of Target Repository changes derived from the current Contract, including Gitignore entries, the Sync manifest, source file bytes, and Target Repository state, including the root `.gitignore` partial region.
_Avoid_: Migration, transaction log.

## Example Dialogue

Dev: The Contract changed `github-workflow.ci` from a Template file to a Managed file.

Domain expert: Then the Sync plan should treat the existing target path as target-owned unless the Sync manifest already records that File key.

Dev: If it is target-owned, driftline reports a conflict instead of overwriting it.

Domain expert: Right. The Sync manifest is the only current record of Managed files; there is no lock file or historical ownership state.
