# AGENTS.md — KFX→EPUB Port

## Work Rules

- Commit after every step — success OR failure, there must always be a commit
- Never accumulate more than one step of uncommitted changes
- If a change introduces new UNEXPECTED test failures or *unexpected* diffs: revert immediately, commit the revert, then figure out why. Regressions are only allowed as temporary artifacts if they are part of a refactor that makes progress on the task. If you cannot explicitly say that they are and why, you must revert it.
- When making plans, You must add specific locations in the python code that we are referencing for each thing we are implementing in go. We are explicitly using the python as the source of truth. File, line, implementation details. Map it 1:1 python to go

## General

- In this workspace, we have a ./REFERENCE/ directory that contains code we or files that we aren't keeping tracked in our repository but we are using to guide our work. Always make sure you have a good idea of the REFERENCE material before you start guessing or making your own decisions on implementation.

- The most important reference we have in this project is the Calibre_KFX_Input/ plugin - It is the sole source of truth for how to interact with KFX files. Every single line our our implementation in Go should be written to map the logic 1:1 into python.

- Our parity should be Three-Fold:
    - 1: Structural. If python has a file in one place, we have a go file with the same name in the same corresponding place in our implementation.
    - 2: Function-level. If python has a function in a particular file, we have a function in go with the same name and same purpose in the same corresponding place.
    - 3: Logic-level. If python's functions do something and return the value 1, our go code's functions will do the same thing in the same places in the same order and return 1.

- We must write tests for every step of this process to ensure each output at each step remains the same, in lockstep with the python code.

- THE ONLY TIME THE PYTHON CAN BE MODIFIED IS FOR DEBUG LOGGING. THE FUNDAMENTAL LOGIC WILL ALWAYS STAY THE SAME IN THE PYTHON.
- WE ARE ONLY IMPLMENTING GO. WE ARE FOLLOWING THE PYTHON AS THE INSTRUCTION MANUAL.
