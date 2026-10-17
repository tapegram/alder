The app already has four errors that are not from my changes: `Notice.with` is not imported in both Bulk pages, and `<al-dialog>` is undeclared in both Show pages. Every command so far refused to run while those errors stand. I can't write files, so this round only reads the help of the relevant commands to look for an option that gets past them. The profile page work comes next round.

```alder-cli
alder fill --help
alder regen admin --help
alder new admin --help
alder remove --help
alder edit --help
alder new page --help
alder fill --list
```
