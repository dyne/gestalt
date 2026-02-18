# [1.16.0](https://github.com/dyne/gestalt/compare/v1.15.0...v1.16.0) (2026-02-18)


### Bug Fixes

* don't rewind to main when starting work ([c0c2943](https://github.com/dyne/gestalt/commit/c0c294345139ed5b1c2a8fcfb6fef07788952d4e))
* hide unfinished agents ([68e9a63](https://github.com/dyne/gestalt/commit/68e9a63aa9ff4cb5acc44106569b163971c6537a))
* update base codex models to 5.3 ([bfef1d8](https://github.com/dyne/gestalt/commit/bfef1d8e1ddb5aa90aaa177b9c6eda9576c94b71))


### Features

* **agent:** add hidden flag ([71342db](https://github.com/dyne/gestalt/commit/71342dbb16faa3d7a3cd2c2f762d8eeae8e5f700))
* **agent:** add model alias support ([d05708b](https://github.com/dyne/gestalt/commit/d05708b70ebe489c4605f2ec7ca81301f5e69b0e))
* **api:** expose hidden agent flag ([4ddd849](https://github.com/dyne/gestalt/commit/4ddd849802776e2db0f8a8d61a6b152ae098b2ec))
* **app:** refresh after external agent create ([f98f100](https://github.com/dyne/gestalt/commit/f98f1000d3b0898112fc284e815df172408ead69))
* **app:** refresh agents tab on terminal events ([00a381b](https://github.com/dyne/gestalt/commit/00a381b0632667a3f6e3a0dc923f3506f6273797))
* **dashboard:** compact agent buttons ([50c27a4](https://github.com/dyne/gestalt/commit/50c27a4dfb1b552b400d1a835c72fe863649acc8))
* **dashboard:** honor hidden agents ([68f97d6](https://github.com/dyne/gestalt/commit/68f97d6f5b7b5dd52fad90a77416f34ba4473e77))
* **frontend:** normalize model fields ([83e5cb2](https://github.com/dyne/gestalt/commit/83e5cb2c9b21a9b6501c9713ab5a20383a64739e))

# [1.15.0](https://github.com/dyne/gestalt/compare/v1.14.0...v1.15.0) (2026-02-18)


### Bug Fixes

* **api:** add missing logging import in rest test ([d3441d6](https://github.com/dyne/gestalt/commit/d3441d616f6ccf3915124e338f79dd293569f1d8))
* **api:** update notify tests for notification sink behavior ([f469946](https://github.com/dyne/gestalt/commit/f4699461c4cf84efb7053f7e60c30e41852bc781))
* **server:** stop agents tmux session on shutdown ([4d0a7da](https://github.com/dyne/gestalt/commit/4d0a7da256f5ae83a56f34b3531ee454b9f26f26))


### Features

* **api:** emit structured notify logs for accepted events ([8679292](https://github.com/dyne/gestalt/commit/8679292e29569d47002f1436af60e0d38bdf1890))
* **dashboard:** expand recent log density with notify chips ([380e9b6](https://github.com/dyne/gestalt/commit/380e9b6947867608856ba791bdfd7d6f0da7c2da))

# [1.14.0](https://github.com/dyne/gestalt/compare/v1.13.0...v1.14.0) (2026-02-18)


### Bug Fixes

* **api:** return 404 for unknown api paths ([d237c49](https://github.com/dyne/gestalt/commit/d237c49b28ed63be1c89256222906887174d3b2c))
* **api:** return 404 for workflow routes ([cebb058](https://github.com/dyne/gestalt/commit/cebb0588e594698d6e80db4349eb9928fdd246a0))
* **frontend:** close agents tab when tmux hub connection fails ([4c39371](https://github.com/dyne/gestalt/commit/4c393718e209e748f24a313817015152904ea52a))
* no need for temporal cleanup check in CI ([efcc616](https://github.com/dyne/gestalt/commit/efcc6164d618f72c46e1e267da8c55c5ad4dfb08))


### Features

* **api:** drop workflow routes ([61e862c](https://github.com/dyne/gestalt/commit/61e862c5a83e8ae3260171ef25059bdd7407ce78))
* **notify:** add notification sink ([25dff13](https://github.com/dyne/gestalt/commit/25dff13fed03f349e033bea537a62d551a3fe2ed))
* **notify:** stream notifications via otel ([1d45511](https://github.com/dyne/gestalt/commit/1d45511215e1c7997bcfabe4a9cdf2a9011f390f))
* **server:** remove temporal runtime flags ([b734592](https://github.com/dyne/gestalt/commit/b73459253cdefd5f111c0a4fbf728a794b9cb200))

# Unreleased

### Breaking Changes

| Area | Change | Compatibility |
| --- | --- | --- |
| Workflows | Remove `/api/workflows`, `/api/workflows/events`, `/api/sessions/:id/workflow/resume`, `/api/sessions/:id/workflow/history`. | Requests now return `404 Not Found`; update clients to rely on flow config + OTel-backed notifications. |
| Status payload | Remove Temporal status fields and workflow flags. | Consumers must ignore `temporal_*` fields and `workflow` session options. |
| Notifications | `/api/notifications/stream` remains, but events are sourced from OTel. | Existing clients can keep the SSE URL unchanged. |

### Migration checklist

- Delete `.gestalt/temporal` (no longer used for local workflow state).
- Remove `GESTALT_TEMPORAL_*` environment variables and `--temporal-*` CLI flags.
- Drop `workflow` from `POST /api/sessions` payloads and remove `use_workflow` from agent TOML.
- Confirm notify consumers read from `/api/notifications/stream` or `/api/otel/logs`.

# [1.13.0](https://github.com/dyne/gestalt/compare/v1.12.0...v1.13.0) (2026-02-18)


### Bug Fixes

* adjust agents close and temporal link ([7bb9463](https://github.com/dyne/gestalt/commit/7bb9463f8a3eb421e3a547a5daa3b332eea0bcfd))
* **cli:** preserve https and gate session logs ([4c8211e](https://github.com/dyne/gestalt/commit/4c8211e8ae24d4edf5e1e467ea9ab043947f2a6b))
* **frontend:** correct temporal URL trailing-slash regex ([b26e8cd](https://github.com/dyne/gestalt/commit/b26e8cd203595b39232fc6dee3784592747431eb))
* **frontend:** refresh agents tab visibility ([a309e27](https://github.com/dyne/gestalt/commit/a309e27d5f9050205047be14d3e99b0eff5ba998))
* gate agents tab on hub status ([d8b4d41](https://github.com/dyne/gestalt/commit/d8b4d416c5fa2fccffdfabf4ea0582eeda52b213))
* hide xterm scrollbar in agents view ([feb9d64](https://github.com/dyne/gestalt/commit/feb9d64ca3cceb6928453021828c56c8547a843c))
* normalize mcp input and tmux attach ([6eb7953](https://github.com/dyne/gestalt/commit/6eb7953aeb34d2042c99810df3f59dc63d6b218e))
* point agents temporal button to workflows ([827d46d](https://github.com/dyne/gestalt/commit/827d46d4feb4fbd158f2b5c03d6e566c14ed03af))
* refresh agents hub on cli create ([bf37ae9](https://github.com/dyne/gestalt/commit/bf37ae9c0bf1941e5411a0be05eb15f219d98b41))
* refresh status after cli session start ([15bf5a5](https://github.com/dyne/gestalt/commit/15bf5a56ef2ac6d4e29937ae26f0908a7abd0d9f))
* resize tmux xterm after connect ([49a940c](https://github.com/dyne/gestalt/commit/49a940ccc40d174115b8931c3a2a35592ffb5c76))
* start tmux for gui-created cli sessions ([adf1951](https://github.com/dyne/gestalt/commit/adf1951c6df6124f699bc1aed2d6d2568294366b))
* **tests:** disable default external tmux startup under go test ([03ec0b2](https://github.com/dyne/gestalt/commit/03ec0b2e6b24cbc143e0199552198a4e1e740af4))


### Features

* **api:** add raw session input endpoint ([3d26796](https://github.com/dyne/gestalt/commit/3d26796233a0c429b26012ca42044dac4df63f43))
* **api:** add tmux window activation endpoint ([6eedc2c](https://github.com/dyne/gestalt/commit/6eedc2cfeeab3bc25a96171df48fbac7df683d5e))
* **cli:** add gestalt-send host/port and session-id modes ([9763eca](https://github.com/dyne/gestalt/commit/9763eca174c33359c6b71fce5e70a30140cd9a74))
* **cli:** switch gestalt-agent to host/port and tmux attach ([1e8377d](https://github.com/dyne/gestalt/commit/1e8377df60f10062d7c8cecbf9990068d9729510))
* **frontend:** activate tmux window when selecting external agents ([c408a95](https://github.com/dyne/gestalt/commit/c408a95e98ad1cd738c2e75e87e1e6b920a420a3))
* **frontend:** add agents hub terminal input mode ([2538967](https://github.com/dyne/gestalt/commit/2538967b223b1430410923c23c525880f687e196))
* **frontend:** add agents tab and hide external cli tabs ([955f7c9](https://github.com/dyne/gestalt/commit/955f7c9a5105d5093e27c961d0c0bb457b2736cd))
* prune closed tmux agent sessions ([c98586f](https://github.com/dyne/gestalt/commit/c98586f4b69f696e6469d4906342aeecc9b62667))
* **runner:** extract shared tmux session helpers ([b1724af](https://github.com/dyne/gestalt/commit/b1724af64bf19c45412eb445b36ac5cd014b5d41))
* **terminal:** create and expose agents tmux hub session ([568f18a](https://github.com/dyne/gestalt/commit/568f18abae0753c627a8928eca2425aa14b10ff8))
* **terminal:** start tmux windows for external cli sessions ([a4e2f86](https://github.com/dyne/gestalt/commit/a4e2f861453527da8e1c7d77b383d5d032168a41))
* **ui:** prefetch plans list on startup ([303b7e7](https://github.com/dyne/gestalt/commit/303b7e75057a6c9a11f56584400ae801000c4166))

# [1.12.0](https://github.com/dyne/gestalt/compare/v1.11.0...v1.12.0) (2026-02-13)


### Bug Fixes

* **logging:** reduce temporal noise and session id duplication ([ddd69e1](https://github.com/dyne/gestalt/commit/ddd69e11f5a68ac4b811fd6eeea264c6e8ba5194))
* **prompt:** improve notify and git instructions ([7c484d3](https://github.com/dyne/gestalt/commit/7c484d388a261531303472f9b53226bd04eaa027))
* **tmux:** correct external session attach instructions ([ece76a4](https://github.com/dyne/gestalt/commit/ece76a49b939c7923ce081866877e1e6b0a711e5))


### Features

* **mcp-cli:** stabilize mcp sessions and remove runner bridge ([a482916](https://github.com/dyne/gestalt/commit/a48291633928ac78e3d1218399ea0f808a3d5736))

# [1.11.0](https://github.com/dyne/gestalt/compare/v1.10.0...v1.11.0) (2026-02-13)


### Bug Fixes

* **ui:** guard lazy-loaded view imports ([ba0e512](https://github.com/dyne/gestalt/commit/ba0e5125903a67eab9ae658234eabbbe944d1b66))


### Features

* **ui:** defer terminal store import ([4ffbe48](https://github.com/dyne/gestalt/commit/4ffbe48450d22b938b171b9121a314f11a1e70bc))
* **ui:** lazy-load tab views ([f3416d7](https://github.com/dyne/gestalt/commit/f3416d7e4fcb3140ab39d1dba15d8915fc070828))
* **vite:** split xterm chunk ([bbf588e](https://github.com/dyne/gestalt/commit/bbf588e6e92044dc46340a6f75fce9b53d19a8ba))

# [1.10.0](https://github.com/dyne/gestalt/compare/v1.9.0...v1.10.0) (2026-02-12)


### Bug Fixes

* **gestalt-agent:** adjust tmux window handling ([bf6a895](https://github.com/dyne/gestalt/commit/bf6a895dd985a04e0ccbdbf6a71d9ec20beacc17))
* **gestalt-agent:** avoid double-escaped runner URLs ([f0d7f1f](https://github.com/dyne/gestalt/commit/f0d7f1f4dd4f764b896f8856752cba1c74463eeb))
* **gestalt-agent:** show tmux attach hint ([eb1593a](https://github.com/dyne/gestalt/commit/eb1593a73707c6496ebd8e175f02f92e62dfa4f3))
* **prompt:** improve prompts and notify behavior ([e55b730](https://github.com/dyne/gestalt/commit/e55b730b80f3efb47c7bc9690906dd3aa0a249c5))
* recycle agent session ids on close ([73c13db](https://github.com/dyne/gestalt/commit/73c13db6f99a5719a1f698fbaabe9fb3c661e939))
* run gestalt-agent via tmux runner ([7d79f40](https://github.com/dyne/gestalt/commit/7d79f4049c2eecbdde6624da4c2ee63d0a9e5790))
* use tmux session target in runner bridge ([090de2d](https://github.com/dyne/gestalt/commit/090de2df72f7e699494ba2e7d4ca95ccdaaf5cd6))


### Features

* add external session runner support ([bb1fbbd](https://github.com/dyne/gestalt/commit/bb1fbbd077bd81eb87ac1d0077aa4eab42077154))
* add runner ws bridge ([867ba01](https://github.com/dyne/gestalt/commit/867ba01626011f67507f2d5257f8732a745a80d7))
* add runner ws protocol types ([4037c97](https://github.com/dyne/gestalt/commit/4037c977b07921ddde8d28539fde5c4c574c0f76))
* add terminal runner abstraction ([8f2d186](https://github.com/dyne/gestalt/commit/8f2d186c4ccf9fba0b288ef844dbe900978e33cd))
* add tmux runner adapter ([6ec1830](https://github.com/dyne/gestalt/commit/6ec183022eaa8c7bbfad6b4f411e5c4254d04e44))
* bridge tmux io over runner ws ([457e342](https://github.com/dyne/gestalt/commit/457e342363cc974d7c7e153d3d453a1bb3c02d4c))
* create external sessions via server ([4cf06b8](https://github.com/dyne/gestalt/commit/4cf06b8bf938684b873b4871f30621f2ae1672f2))
* **frontend:** add console module ([3235f7b](https://github.com/dyne/gestalt/commit/3235f7b69aeca18cd911e711fa6452893ec93453))
* **frontend:** resolve gui modules ([b9209b9](https://github.com/dyne/gestalt/commit/b9209b9f850b294e52071c224f4ed357df728f15))
* share codex argv and launch spec ([7f98176](https://github.com/dyne/gestalt/commit/7f98176b5e1976a46174801139990b973e82cf50))

# [1.9.0](https://github.com/dyne/gestalt/compare/v1.8.0...v1.9.0) (2026-02-12)


### Features

* **agent:** accept gui_modules in toml ([eeec650](https://github.com/dyne/gestalt/commit/eeec650de0c1783c409d49d1798fd67cc14be0a3))
* **agent:** add gui-modules to sessions ([18cab5a](https://github.com/dyne/gestalt/commit/18cab5a229382cf78843a54ca7349682cfce7d18))
* **api:** add session progress endpoint ([474530c](https://github.com/dyne/gestalt/commit/474530c6a2e63327884d5ecb9ce42e44e6653327))
* **api:** normalize progress notify payload ([538f8a0](https://github.com/dyne/gestalt/commit/538f8a022399251a855987ef131839e3bd161ff7))
* **api:** publish progress terminal events ([fc68286](https://github.com/dyne/gestalt/commit/fc68286f1b431289b5f02c2fcf9c23b91edca7dd))
* **plan:** extract body text from source ([1a5b56e](https://github.com/dyne/gestalt/commit/1a5b56e0a292d3868310b3403e97a0fad5ef54e5))
* **ui:** add plan progress sidebar ([aff4e5d](https://github.com/dyne/gestalt/commit/aff4e5dec2c30c930bea33238d00ed074ebd1d17))
* **ui:** add plan toggle for coder sessions ([c5cc38a](https://github.com/dyne/gestalt/commit/c5cc38a89336d71f2a5da8b504055d30b4516aad))
* **ui:** gate plan toggle by gui-modules ([6836be5](https://github.com/dyne/gestalt/commit/6836be58ad3cd10edfded293e3a9cfdf35608397))
* **ui:** pass gui modules to terminals ([fa1683b](https://github.com/dyne/gestalt/commit/fa1683b5f3ec9048ed91388b33ca113c8bde98bc))

# [1.8.0](https://github.com/dyne/gestalt/compare/v1.7.1...v1.8.0) (2026-02-12)


### Bug Fixes

* **prompt:** improve coder agent ([acb20f9](https://github.com/dyne/gestalt/commit/acb20f9894643f51ce3e8de4ae230d9ee9279f10))


### Features

* **api:** route notify into flow ([5a20e11](https://github.com/dyne/gestalt/commit/5a20e117bb3479900d38ccadf7c5afd8e9044112))
* **flow:** add config export and import ([95cf90a](https://github.com/dyne/gestalt/commit/95cf90a724f7f04bb7ff46e0531d2ec3e77a9fa8))
* **flow:** add event signaling ([b61cc92](https://github.com/dyne/gestalt/commit/b61cc925685968357b3f5e557440d841fa4af672))
* **flow:** add notify trigger preset ([746b080](https://github.com/dyne/gestalt/commit/746b08039f0496cd57eb26aabfeb37431e634d54))
* **flow:** add template helpers ([61d1d4d](https://github.com/dyne/gestalt/commit/61d1d4dfc6ddbbe1efb24972ee72a1b7218b4ffc))
* **flow:** canonicalize notify events ([2ffa0ed](https://github.com/dyne/gestalt/commit/2ffa0ed3b0145968387d39637ed3945324195e95))
* **flow:** clarify session targeting ([26c3fa0](https://github.com/dyne/gestalt/commit/26c3fa019afe2dc8dbc125d4af6b13a177871479))
* **flow:** fetch event types from api ([fa66072](https://github.com/dyne/gestalt/commit/fa6607282c661b5a5732fc1a34839aad472b15ce))
* **flow:** normalize notify fields ([d1eaff4](https://github.com/dyne/gestalt/commit/d1eaff46b4a86d1e2c089645e2dad9575f3f1c9f))
* **flow:** render templates in activities ([29070b6](https://github.com/dyne/gestalt/commit/29070b600ec559d36d34fa54b305ae1f905fa783))
* **flow:** spawn agent sessions ([516c022](https://github.com/dyne/gestalt/commit/516c022c9929076b7a8a2ddd0836ad309310f603))
* **flow:** target sessions by id ([401caf4](https://github.com/dyne/gestalt/commit/401caf42a665c47f7f915426c242111707c6a479))
* **temporal:** add flow dispatch workflow ([6d83328](https://github.com/dyne/gestalt/commit/6d833284b09392b2d25015212b1db508798f7214))
* **temporal:** register flow dispatch workflow ([fa139a7](https://github.com/dyne/gestalt/commit/fa139a7eb5ca8788113721d97bd5f90656f56820))
* **temporal:** spawn flow child workflows ([f3b44cd](https://github.com/dyne/gestalt/commit/f3b44cd7661ae9d5f9b18f8f6e94716bd47f006e))
* **ui:** add notify flow event types ([c95f984](https://github.com/dyne/gestalt/commit/c95f9845774bdda9ddfc217d6699140ed3198b81))

## [1.7.1](https://github.com/dyne/gestalt/compare/v1.7.0...v1.7.1) (2026-02-11)


### Bug Fixes

* version inside released binaries ([84a190c](https://github.com/dyne/gestalt/commit/84a190c033825a3faab50a78d4ea9101fd3f736e))

# [1.7.0](https://github.com/dyne/gestalt/compare/v1.6.2...v1.7.0) (2026-02-11)


### Bug Fixes

* add process registry ([e6e0032](https://github.com/dyne/gestalt/commit/e6e00322498cd0e0f8e5642e310b1518a393b7fc))
* add shutdown coordinator ordering ([1f035b9](https://github.com/dyne/gestalt/commit/1f035b92fecc83fe5a5507841974e24cf1411ccc))
* add shutdown timeouts ([52cd31f](https://github.com/dyne/gestalt/commit/52cd31f466bb8a6c2767757a4a6ceaec0d19db6d))
* adjust codex prompt test timing ([fb2bfee](https://github.com/dyne/gestalt/commit/fb2bfeeb022d855dd55e251d0b899329db23cf68))
* close sessions on shutdown ([247485a](https://github.com/dyne/gestalt/commit/247485a043fb063b9108de622e01113447d83e95))
* force cli codex in instructions test ([86537bf](https://github.com/dyne/gestalt/commit/86537bf3b09ee17ca4301317e392652f105317b7))
* improve defaults of copilot cli agent ([437733b](https://github.com/dyne/gestalt/commit/437733b0a8bc32250003c2fee63449a80af6ae76))
* improve prompts for architect and coder ([c694beb](https://github.com/dyne/gestalt/commit/c694bebd4b60717d23eb6d6b001c85dd1317346f))
* manage pty process groups ([93c8bc0](https://github.com/dyne/gestalt/commit/93c8bc0ec5072a1681ec592f93c8db393447d0dd))
* normalize mcp input handling ([0b94905](https://github.com/dyne/gestalt/commit/0b949057d553a79c0dd1bf5fefbdcadab1aacd6f))
* normalize mcp output newlines ([d42e836](https://github.com/dyne/gestalt/commit/d42e83624c7ae3e92954d71e7dda06f8b449f6e4))
* relax websocket test timeout ([bc4df08](https://github.com/dyne/gestalt/commit/bc4df08f259decce2cf57c38501b24ee6525e5f3))
* render terminal canvas slots ([fe37c2d](https://github.com/dyne/gestalt/commit/fe37c2d4b8b2e9fec19b63eccf01dbe4544a68cc))
* resolve merge fallout ([95283e7](https://github.com/dyne/gestalt/commit/95283e7899e295edb05f7dd00722e3db13adc8fe))
* stabilize cli session start ([c3487c2](https://github.com/dyne/gestalt/commit/c3487c2bbd8395440a141205c734ce6b67f4c4dc))
* stabilize concurrent ws test ([0e0c256](https://github.com/dyne/gestalt/commit/0e0c256939a4fcb201ddf45ebc067c3ec24d6b1b))
* stabilize terminal ui state ([3d91e4b](https://github.com/dyne/gestalt/commit/3d91e4be2acb500658eaa80da9ea27fb317bc8f3))
* stabilize websocket integration tests ([5f72ae4](https://github.com/dyne/gestalt/commit/5f72ae4cf5c1c0cdc42494999c232a0adb5d3cc5))
* update codex integration expectations ([49e93d7](https://github.com/dyne/gestalt/commit/49e93d76e74413c4120050c185e9655776777d2f))
* update prompts and instruct gestalt-notify ([c7e4347](https://github.com/dyne/gestalt/commit/c7e434715becaff5bfc23a5ac29fe6594e0b8830))
* update shutdown signal handling ([19419c5](https://github.com/dyne/gestalt/commit/19419c58672038f52b41458cf5b20b5e0bdfb306))
* version flag now shows git latest tag ([2dc3626](https://github.com/dyne/gestalt/commit/2dc362635291038c8d79d7eb0755e5a67eafb828))
* version in tabbar ([717ce8e](https://github.com/dyne/gestalt/commit/717ce8ef72548465ba3f69fac503cf968d4e6945))


### Features

* add agent interface invariants ([5404cbc](https://github.com/dyne/gestalt/commit/5404cbc249a0306e53aba131580bb7619c813ab0))
* add codex_mode to agents ([2827892](https://github.com/dyne/gestalt/commit/2827892493906bb0163e8cc0ab4fd3536c82bebf))
* add developer instructions helper ([a53f7de](https://github.com/dyne/gestalt/commit/a53f7de5682211bdba72d6f959eca731f1ba6e19))
* add full copilot example config ([b813a5a](https://github.com/dyne/gestalt/commit/b813a5acaa0febfdeb6d46ceb915505c716742b0))
* add input font settings ([1eb945d](https://github.com/dyne/gestalt/commit/1eb945d228d31116fe5f5462243670477fe5b157))
* add input resize handle ([c16b81e](https://github.com/dyne/gestalt/commit/c16b81e41134bea2d7c85a6e2018639f73fa3436))
* add log-codex-events setting ([335d687](https://github.com/dyne/gestalt/commit/335d687f34798b54908e21a52f7a67c3fa4cda57))
* add mcp pty adapter ([9787234](https://github.com/dyne/gestalt/commit/97872347ce0c3a2db429172dffee4c9d8a9c62ae))
* add prompt output segments ([9fda76f](https://github.com/dyne/gestalt/commit/9fda76f119271c06c0e13273f9f003fce9949f43))
* add session header controls ([d6a51b2](https://github.com/dyne/gestalt/commit/d6a51b2b519536baaf7314a80a94d7f3e0b8c7a3))
* add stdio pty factory ([9a6e47e](https://github.com/dyne/gestalt/commit/9a6e47e8a8304026ea29c5bad1e8f02b710d8528))
* add terminal text view ([0dc078b](https://github.com/dyne/gestalt/commit/0dc078b2101e7c715de9fbefcca376205e4b9e09))
* add text buffer module ([925216a](https://github.com/dyne/gestalt/commit/925216a987ac3fb4dc45df73bc034779758feb05))
* decouple terminal socket ([da12ac4](https://github.com/dyne/gestalt/commit/da12ac4d662b5b71daf520448a2527cc8837fd16))
* emit mcp notify signals ([5271fee](https://github.com/dyne/gestalt/commit/5271feeda0ce7221d9d14b7926580fc0d30d02c5))
* expose agent interface ([ab2dcf0](https://github.com/dyne/gestalt/commit/ab2dcf027ab30e3d1b9976431e537fa3925a253e))
* expose session interface ([27bc3dd](https://github.com/dyne/gestalt/commit/27bc3ddcf6c493b8e8262ec097dd1c1af1224ab6))
* expose session ui config ([49b6718](https://github.com/dyne/gestalt/commit/49b671802c5337065dc00fd20f2ff6d5ab2870bf))
* fit terminal view on mobile ([1339f1e](https://github.com/dyne/gestalt/commit/1339f1effae70045610cf1a5567565dc4088708d))
* format mcp notifications in output ([921329f](https://github.com/dyne/gestalt/commit/921329fe79bb554af922f8eb0004d4c54e90210e))
* handle carriage returns ([c25151b](https://github.com/dyne/gestalt/commit/c25151bbfa2a0711d8eae42c9c9e5dc690fa3e02))
* hash agent interface ([b506766](https://github.com/dyne/gestalt/commit/b506766cd8013b8717f1cfd809d239616f77e897))
* log mcp events to file ([ae7f0f9](https://github.com/dyne/gestalt/commit/ae7f0f9c2a2aec0d1085d86223f2fc3989708e0f))
* make mcp prompt injection safe ([4075209](https://github.com/dyne/gestalt/commit/4075209a0fa05cfa97a248ef2e169a0d98062dd0))
* remove xterm session view ([87cc800](https://github.com/dyne/gestalt/commit/87cc80046ef41673a0fb1b597b771217227e6388))
* restore xterm terminal service ([8302c7a](https://github.com/dyne/gestalt/commit/8302c7adf77294bfff5b37b56537bd53477c206c))
* route codex mcp sessions ([4d61c31](https://github.com/dyne/gestalt/commit/4d61c31b2d5c34ae21b3a74956b2313ff56bb81f))
* route mcp notifications in read loop ([08143af](https://github.com/dyne/gestalt/commit/08143afc1b674636eb6cf83411d1ed0b74ddf8e1))
* select session interface ([cc6dfb6](https://github.com/dyne/gestalt/commit/cc6dfb662c7d20fe6af0a526176b797775328ac2))
* suppress prompt echo ([1328940](https://github.com/dyne/gestalt/commit/132894084b6689df997fc0e29cac9d61267052bb))
* sync xterm ui config ([8cc65e2](https://github.com/dyne/gestalt/commit/8cc65e21e4e65339d9b33f5f4fe49a0f557c52c2))
* unify gestalt-agent instructions ([0f5f0d5](https://github.com/dyne/gestalt/commit/0f5f0d5d8874a0d84290dcf74137562b3200baf1))
* update embedded agent interfaces ([5cd91f9](https://github.com/dyne/gestalt/commit/5cd91f9ac46e3b62dc107ab2e85bbeb683c3399e))
* wire codex developer instructions ([0c1a037](https://github.com/dyne/gestalt/commit/0c1a0375ddabb5388f39ee0c06c283a50b3d5bd2))
* wire session interface in terminal UI ([12e4651](https://github.com/dyne/gestalt/commit/12e46512912a8ed9a0805d3482049a960a3df01c))

## [1.6.2](https://github.com/dyne/gestalt/compare/v1.6.1...v1.6.2) (2026-02-03)


### Bug Fixes

* **release:** add gestalt-agent to release ([f4e60f3](https://github.com/dyne/gestalt/commit/f4e60f31bde18b10dd21e30b978b5529cd28521e))

## [1.6.1](https://github.com/dyne/gestalt/compare/v1.6.0...v1.6.1) (2026-02-03)


### Bug Fixes

* build makefile install typo ([0154f25](https://github.com/dyne/gestalt/commit/0154f25c78a71ea3360f5a5b63d6eb48dae1cff8))

# [1.6.0](https://github.com/dyne/gestalt/compare/v1.5.0...v1.6.0) (2026-02-03)


### Bug Fixes

* add notify error codes ([09ad927](https://github.com/dyne/gestalt/commit/09ad92757a2a335d5ea1650a6e46747bd5714a63))
* drop notify agent id flag ([d587458](https://github.com/dyne/gestalt/commit/d58745847d3685a445db9806f05da49bf325658a))
* drop notify agent id support ([10dcaf5](https://github.com/dyne/gestalt/commit/10dcaf50641c5b299c13764b0d906d854c19e14a))
* drop notify agent name/source ([35e3771](https://github.com/dyne/gestalt/commit/35e377165e866df4226973a00e078dfc3915facf))
* escape notify session id ([f9c5a2e](https://github.com/dyne/gestalt/commit/f9c5a2eb1b2e273b40ce12138bed3b38fea9cac7))
* gestalt-agent uses developer_instructions ([951e50d](https://github.com/dyne/gestalt/commit/951e50d8af89258dc7f5386b4f856b9fde9ffcb6))
* infer notify agent id ([8489193](https://github.com/dyne/gestalt/commit/848919336bdfba6b0f9e5c6fa8f6ee2eae67eac9))
* make notify agent id optional ([ec031d3](https://github.com/dyne/gestalt/commit/ec031d38de71cb1cd7c9e44b889599c6960bb00e))
* omit empty notify agent id ([21e5c4e](https://github.com/dyne/gestalt/commit/21e5c4eefa2b657f8900649e5efdeeca0dd8ec9a))
* relax notify agent id validation ([722cdb1](https://github.com/dyne/gestalt/commit/722cdb16946352bcac302388abf7e9de388d3483))
* require notify payload type ([afcae39](https://github.com/dyne/gestalt/commit/afcae39202ba5a14b8bca5602395b2c04dd83515))
* use positional notify payload ([585bb44](https://github.com/dyne/gestalt/commit/585bb444e503b92697613714bf880655d654b81b))


### Features

* add inline prompt directives ([44b3a42](https://github.com/dyne/gestalt/commit/44b3a4295f9a89ff1f1012c75a0ed13de7d52d4f))
* inject session id in prompts ([6e4fa59](https://github.com/dyne/gestalt/commit/6e4fa59924ce2c3d54b4bbc75a93c7b2be54996e))

# [1.5.0](https://github.com/dyne/gestalt/compare/v1.4.0...v1.5.0) (2026-02-03)


### Features

* gestalt-agent to open session on console ([8948fc2](https://github.com/dyne/gestalt/commit/8948fc2c43cf3a91d1cfaa5136dc5669c2e26e50))

# [1.4.0](https://github.com/dyne/gestalt/compare/v1.3.1...v1.4.0) (2026-02-02)


### Features

* add conffile decision table ([e440d29](https://github.com/dyne/gestalt/commit/e440d29a3f8b50a148f57abaee5e3a30332d7608))
* add conffile prompt UI ([154fd1f](https://github.com/dyne/gestalt/commit/154fd1f11b045cf5bfaba905ef0bb15efdbbc655))
* add config overrides env ([cfb3103](https://github.com/dyne/gestalt/commit/cfb310368677d36fc0e41e271e67c430bba34f00))
* add config overrides flag ([21a1bb1](https://github.com/dyne/gestalt/commit/21a1bb1b5b8b09acaa5c9c699933ac48f4cce256))
* add gestalt.toml defaults ([40087d5](https://github.com/dyne/gestalt/commit/40087d5496963b9376ab8e5d27e692189ea9b6b3))
* add toml key store ([6ecb106](https://github.com/dyne/gestalt/commit/6ecb106451be67073d91416243b812a1d7c7d215))
* default conffile keep in non-tty ([81e53fd](https://github.com/dyne/gestalt/commit/81e53fd63b08e757746d18a0bd2e7c471b457978))
* load gestalt.toml path ([026714d](https://github.com/dyne/gestalt/commit/026714d01a31e1651a3301ca5a4f5629560eb222))
* save gestalt toml defaults ([19f62d7](https://github.com/dyne/gestalt/commit/19f62d7f69f0f7f20bb5975723dbc0b0cec7ebfb))
* serialize interactive config extraction ([8879346](https://github.com/dyne/gestalt/commit/88793467c9dc4ce2759875748c5a35a93940569c))
* share toml parsing ([c04fa68](https://github.com/dyne/gestalt/commit/c04fa68d1faf993c00eac8a403bb5b9e8e041484))
* track config baseline manifest ([7ff5421](https://github.com/dyne/gestalt/commit/7ff54217929f3576610667db1d6899a066453cf3))
* wire gestalt settings into runtime limits ([1179fde](https://github.com/dyne/gestalt/commit/1179fde485ec982dbdb89d4436f99aa7124ef3d0))
* write dist sidecars on keep ([7253cb5](https://github.com/dyne/gestalt/commit/7253cb57474f18398806a2828930927ea0753a51))

## [1.3.1](https://github.com/dyne/gestalt/compare/v1.3.0...v1.3.1) (2026-01-31)


### Bug Fixes

* make go clean -cache ([427c98c](https://github.com/dyne/gestalt/commit/427c98c9215cde21b9da7995994c3ebc811efa11))

# [1.3.0](https://github.com/dyne/gestalt/compare/v1.2.0...v1.3.0) (2026-01-30)


### Bug Fixes

* bound otel collector stderr ([868ebc7](https://github.com/dyne/gestalt/commit/868ebc7e8ff89ed613b01899fc18518a4af7d6d3))
* curb otel exporter spam ([a37c69f](https://github.com/dyne/gestalt/commit/a37c69f48ab75b4359fd0f3c44845e2e7ce55241))
* expose otel collector status ([4babb95](https://github.com/dyne/gestalt/commit/4babb95b16d7db24a0409a7b60408da5434895db))
* gate otel collector readiness ([aff1276](https://github.com/dyne/gestalt/commit/aff1276cb1f97a54d871b6882172e7a3f396cfc5))
* install gestalt-otel ([9e00ac5](https://github.com/dyne/gestalt/commit/9e00ac51d323388cd4797945fb25d22c2e2a5a42))
* prompt improvements ([537ef14](https://github.com/dyne/gestalt/commit/537ef14139abad6ccba13301c4fd611306458d87))
* report temporal dev server status ([ef721e6](https://github.com/dyne/gestalt/commit/ef721e65afb8ed03f56e5c96e6b32297e85813ce))
* stop daemons on shutdown ([e1cb0eb](https://github.com/dyne/gestalt/commit/e1cb0ebfa49ee2d01a48d29c5a6438e28ad9738f))
* supervise otel collector ([ef1ee3c](https://github.com/dyne/gestalt/commit/ef1ee3c696b8950ea4462b50696d4eaa4ac9e757))
* update prompts and agent instructions ([9bb6dbd](https://github.com/dyne/gestalt/commit/9bb6dbdcd36c608599aa0d29758aa4c7fdd4a5f3))
* use session-id for codex notify ([0f326d8](https://github.com/dyne/gestalt/commit/0f326d882e4f08586a3897a718457654e23032bb))


### Features

* add activity assigner component ([7e9d0c8](https://github.com/dyne/gestalt/commit/7e9d0c81aaf4e969f125a2330875911517e7529b))
* add drag and drop assignment ([c195b6a](https://github.com/dyne/gestalt/commit/c195b6a56168bd1307c39938fba90f3fc17df760))
* add flow activities ([697c5fb](https://github.com/dyne/gestalt/commit/697c5fbefaf4a663797a6d9fa0a9be199f553366))
* add flow config endpoints ([d8d242b](https://github.com/dyne/gestalt/commit/d8d242b86df672f38d9b42ba246bc39ce8bb69eb))
* add flow config repository ([7e1f6e5](https://github.com/dyne/gestalt/commit/7e1f6e5d5d948a55ab9f747653d15fee89975259))
* add flow event matching ([49b9692](https://github.com/dyne/gestalt/commit/49b9692ddde5bb5c990f36b9e2972b7aec84419c))
* add flow idempotency helpers ([bf60a9d](https://github.com/dyne/gestalt/commit/bf60a9d1f85c35ff324114d3af29147c0cba43b5))
* add flow temporal router bridge ([4619c8c](https://github.com/dyne/gestalt/commit/4619c8ccd4ceb27c29e804bf5af5476b34701b7c))
* add flow trigger filtering ([1273db4](https://github.com/dyne/gestalt/commit/1273db44f879527e36c5a8c07518c39ebd3c183c))
* add notification toast stream ([06b09a6](https://github.com/dyne/gestalt/commit/06b09a62440c17cd0ee5970808c01573323541d1))
* add trigger editor dialog ([8fbe3f8](https://github.com/dyne/gestalt/commit/8fbe3f8286ef0e1f122b3e622b9940350dc2ae43))
* align session terminology in UI ([a534bbc](https://github.com/dyne/gestalt/commit/a534bbcf77edd4813f43efe3d3735b8270dcabd4))
* align workflow id with session id ([ffc8b76](https://github.com/dyne/gestalt/commit/ffc8b767d9a74cb71a7af5752838d745054d87ee))
* compact agent dashboard ([4b20242](https://github.com/dyne/gestalt/commit/4b20242895ec90ff55596c2c412a321f42011b3a))
* cross-compile binaries for all platforms ([9d9e77d](https://github.com/dyne/gestalt/commit/9d9e77d07c2ca6cecf3fcd88c0258deb425ca542))
* encode session ids in client paths ([4779c3f](https://github.com/dyne/gestalt/commit/4779c3f1127aa974910934f57c42d0dda91ca6ca))
* gate terminal connections on visibility ([06e7a13](https://github.com/dyne/gestalt/commit/06e7a1365bcb35c48a8573d606f437156b4d4cf9))
* generate agent session ids ([d693552](https://github.com/dyne/gestalt/commit/d693552da4a4b7cd1082cd6a0f2c764a4b780037))
* harden session log cleanup ([e35d6b4](https://github.com/dyne/gestalt/commit/e35d6b4f011cc246b29d079242696126d0d20166))
* label tabs by session id ([dfe7548](https://github.com/dyne/gestalt/commit/dfe7548bce7b9a9af1b3df65611b58979dde6b90))
* lazy mount active terminal view ([1da929b](https://github.com/dyne/gestalt/commit/1da929b6d5be4bb38f1ab24544b605cc155092a6))
* rename session json keys ([c1b7a54](https://github.com/dyne/gestalt/commit/c1b7a542cd46bdb048853d71067d63652d057744))
* rename session routes ([f6006a7](https://github.com/dyne/gestalt/commit/f6006a77e66493943fdb23f910552b55b59aee02))
* rename terminal strings ([1d68aa7](https://github.com/dyne/gestalt/commit/1d68aa75cdaa910618d3f80bc86f496eada6380b))
* show terminal id in header ([198762c](https://github.com/dyne/gestalt/commit/198762cd3fe466a252d309f9542a929f599608ac))
* start flow bridge on boot ([3f5823a](https://github.com/dyne/gestalt/commit/3f5823a49e730d0271b494c9368fcb84a8196be0))
* switch gestalt-notify to session ids ([c5d39ff](https://github.com/dyne/gestalt/commit/c5d39ff5a0efe765cccb0ed31be41b1a31ec2c1c))
* track agent session counters ([5cf16ca](https://github.com/dyne/gestalt/commit/5cf16ca34d13be92b6be914e5f07bc7effd340f3))
* update gestalt-send session start ([c4df161](https://github.com/dyne/gestalt/commit/c4df1610b2c76f31d04903b4e34a9802e59ea0b2))
* wire flow config UI ([ea281ae](https://github.com/dyne/gestalt/commit/ea281ae278ffa05b7e3d2e26e5788d0e67029131))

# [1.3.0](https://github.com/dyne/gestalt/compare/v1.2.0...v1.3.0) (2026-01-29)


### Bug Fixes

* bound otel collector stderr ([868ebc7](https://github.com/dyne/gestalt/commit/868ebc7e8ff89ed613b01899fc18518a4af7d6d3))
* curb otel exporter spam ([a37c69f](https://github.com/dyne/gestalt/commit/a37c69f48ab75b4359fd0f3c44845e2e7ce55241))
* expose otel collector status ([4babb95](https://github.com/dyne/gestalt/commit/4babb95b16d7db24a0409a7b60408da5434895db))
* gate otel collector readiness ([aff1276](https://github.com/dyne/gestalt/commit/aff1276cb1f97a54d871b6882172e7a3f396cfc5))
* install gestalt-otel ([9e00ac5](https://github.com/dyne/gestalt/commit/9e00ac51d323388cd4797945fb25d22c2e2a5a42))
* prompt improvements ([537ef14](https://github.com/dyne/gestalt/commit/537ef14139abad6ccba13301c4fd611306458d87))
* report temporal dev server status ([ef721e6](https://github.com/dyne/gestalt/commit/ef721e65afb8ed03f56e5c96e6b32297e85813ce))
* stop daemons on shutdown ([e1cb0eb](https://github.com/dyne/gestalt/commit/e1cb0ebfa49ee2d01a48d29c5a6438e28ad9738f))
* supervise otel collector ([ef1ee3c](https://github.com/dyne/gestalt/commit/ef1ee3c696b8950ea4462b50696d4eaa4ac9e757))
* update prompts and agent instructions ([9bb6dbd](https://github.com/dyne/gestalt/commit/9bb6dbdcd36c608599aa0d29758aa4c7fdd4a5f3))
* use session-id for codex notify ([0f326d8](https://github.com/dyne/gestalt/commit/0f326d882e4f08586a3897a718457654e23032bb))


### Features

* add activity assigner component ([7e9d0c8](https://github.com/dyne/gestalt/commit/7e9d0c81aaf4e969f125a2330875911517e7529b))
* add drag and drop assignment ([c195b6a](https://github.com/dyne/gestalt/commit/c195b6a56168bd1307c39938fba90f3fc17df760))
* add flow activities ([697c5fb](https://github.com/dyne/gestalt/commit/697c5fbefaf4a663797a6d9fa0a9be199f553366))
* add flow config endpoints ([d8d242b](https://github.com/dyne/gestalt/commit/d8d242b86df672f38d9b42ba246bc39ce8bb69eb))
* add flow config repository ([7e1f6e5](https://github.com/dyne/gestalt/commit/7e1f6e5d5d948a55ab9f747653d15fee89975259))
* add flow event matching ([49b9692](https://github.com/dyne/gestalt/commit/49b9692ddde5bb5c990f36b9e2972b7aec84419c))
* add flow idempotency helpers ([bf60a9d](https://github.com/dyne/gestalt/commit/bf60a9d1f85c35ff324114d3af29147c0cba43b5))
* add flow temporal router bridge ([4619c8c](https://github.com/dyne/gestalt/commit/4619c8ccd4ceb27c29e804bf5af5476b34701b7c))
* add flow trigger filtering ([1273db4](https://github.com/dyne/gestalt/commit/1273db44f879527e36c5a8c07518c39ebd3c183c))
* add notification toast stream ([06b09a6](https://github.com/dyne/gestalt/commit/06b09a62440c17cd0ee5970808c01573323541d1))
* add trigger editor dialog ([8fbe3f8](https://github.com/dyne/gestalt/commit/8fbe3f8286ef0e1f122b3e622b9940350dc2ae43))
* align session terminology in UI ([a534bbc](https://github.com/dyne/gestalt/commit/a534bbcf77edd4813f43efe3d3735b8270dcabd4))
* align workflow id with session id ([ffc8b76](https://github.com/dyne/gestalt/commit/ffc8b767d9a74cb71a7af5752838d745054d87ee))
* compact agent dashboard ([4b20242](https://github.com/dyne/gestalt/commit/4b20242895ec90ff55596c2c412a321f42011b3a))
* encode session ids in client paths ([4779c3f](https://github.com/dyne/gestalt/commit/4779c3f1127aa974910934f57c42d0dda91ca6ca))
* gate terminal connections on visibility ([06e7a13](https://github.com/dyne/gestalt/commit/06e7a1365bcb35c48a8573d606f437156b4d4cf9))
* generate agent session ids ([d693552](https://github.com/dyne/gestalt/commit/d693552da4a4b7cd1082cd6a0f2c764a4b780037))
* harden session log cleanup ([e35d6b4](https://github.com/dyne/gestalt/commit/e35d6b4f011cc246b29d079242696126d0d20166))
* label tabs by session id ([dfe7548](https://github.com/dyne/gestalt/commit/dfe7548bce7b9a9af1b3df65611b58979dde6b90))
* lazy mount active terminal view ([1da929b](https://github.com/dyne/gestalt/commit/1da929b6d5be4bb38f1ab24544b605cc155092a6))
* rename session json keys ([c1b7a54](https://github.com/dyne/gestalt/commit/c1b7a542cd46bdb048853d71067d63652d057744))
* rename session routes ([f6006a7](https://github.com/dyne/gestalt/commit/f6006a77e66493943fdb23f910552b55b59aee02))
* rename terminal strings ([1d68aa7](https://github.com/dyne/gestalt/commit/1d68aa75cdaa910618d3f80bc86f496eada6380b))
* show terminal id in header ([198762c](https://github.com/dyne/gestalt/commit/198762cd3fe466a252d309f9542a929f599608ac))
* start flow bridge on boot ([3f5823a](https://github.com/dyne/gestalt/commit/3f5823a49e730d0271b494c9368fcb84a8196be0))
* switch gestalt-notify to session ids ([c5d39ff](https://github.com/dyne/gestalt/commit/c5d39ff5a0efe765cccb0ed31be41b1a31ec2c1c))
* track agent session counters ([5cf16ca](https://github.com/dyne/gestalt/commit/5cf16ca34d13be92b6be914e5f07bc7effd340f3))
* update gestalt-send session start ([c4df161](https://github.com/dyne/gestalt/commit/c4df1610b2c76f31d04903b4e34a9802e59ea0b2))
* wire flow config UI ([ea281ae](https://github.com/dyne/gestalt/commit/ea281ae278ffa05b7e3d2e26e5788d0e67029131))

# [1.3.0](https://github.com/dyne/gestalt/compare/v1.2.0...v1.3.0) (2026-01-29)


### Features

* add activity assigner component ([7e9d0c8](https://github.com/dyne/gestalt/commit/7e9d0c81aaf4e969f125a2330875911517e7529b))
* add drag and drop assignment ([c195b6a](https://github.com/dyne/gestalt/commit/c195b6a56168bd1307c39938fba90f3fc17df760))
* add flow activities ([697c5fb](https://github.com/dyne/gestalt/commit/697c5fbefaf4a663797a6d9fa0a9be199f553366))
* add flow config endpoints ([d8d242b](https://github.com/dyne/gestalt/commit/d8d242b86df672f38d9b42ba246bc39ce8bb69eb))
* add flow config repository ([7e1f6e5](https://github.com/dyne/gestalt/commit/7e1f6e5d5d948a55ab9f747653d15fee89975259))
* add flow event matching ([49b9692](https://github.com/dyne/gestalt/commit/49b9692ddde5bb5c990f36b9e2972b7aec84419c))
* add flow idempotency helpers ([bf60a9d](https://github.com/dyne/gestalt/commit/bf60a9d1f85c35ff324114d3af29147c0cba43b5))
* add flow temporal router bridge ([4619c8c](https://github.com/dyne/gestalt/commit/4619c8ccd4ceb27c29e804bf5af5476b34701b7c))
* add flow trigger filtering ([1273db4](https://github.com/dyne/gestalt/commit/1273db44f879527e36c5a8c07518c39ebd3c183c))
* add notification toast stream ([06b09a6](https://github.com/dyne/gestalt/commit/06b09a62440c17cd0ee5970808c01573323541d1))
* add trigger editor dialog ([8fbe3f8](https://github.com/dyne/gestalt/commit/8fbe3f8286ef0e1f122b3e622b9940350dc2ae43))
* align session terminology in UI ([a534bbc](https://github.com/dyne/gestalt/commit/a534bbcf77edd4813f43efe3d3735b8270dcabd4))
* align workflow id with session id ([ffc8b76](https://github.com/dyne/gestalt/commit/ffc8b767d9a74cb71a7af5752838d745054d87ee))
* compact agent dashboard ([4b20242](https://github.com/dyne/gestalt/commit/4b20242895ec90ff55596c2c412a321f42011b3a))
* encode session ids in client paths ([4779c3f](https://github.com/dyne/gestalt/commit/4779c3f1127aa974910934f57c42d0dda91ca6ca))
* generate agent session ids ([d693552](https://github.com/dyne/gestalt/commit/d693552da4a4b7cd1082cd6a0f2c764a4b780037))
* harden session log cleanup ([e35d6b4](https://github.com/dyne/gestalt/commit/e35d6b4f011cc246b29d079242696126d0d20166))
* label tabs by session id ([dfe7548](https://github.com/dyne/gestalt/commit/dfe7548bce7b9a9af1b3df65611b58979dde6b90))
* rename session json keys ([c1b7a54](https://github.com/dyne/gestalt/commit/c1b7a542cd46bdb048853d71067d63652d057744))
* rename session routes ([f6006a7](https://github.com/dyne/gestalt/commit/f6006a77e66493943fdb23f910552b55b59aee02))
* rename terminal strings ([1d68aa7](https://github.com/dyne/gestalt/commit/1d68aa75cdaa910618d3f80bc86f496eada6380b))
* show terminal id in header ([198762c](https://github.com/dyne/gestalt/commit/198762cd3fe466a252d309f9542a929f599608ac))
* start flow bridge on boot ([3f5823a](https://github.com/dyne/gestalt/commit/3f5823a49e730d0271b494c9368fcb84a8196be0))
* switch gestalt-notify to session ids ([c5d39ff](https://github.com/dyne/gestalt/commit/c5d39ff5a0efe765cccb0ed31be41b1a31ec2c1c))
* track agent session counters ([5cf16ca](https://github.com/dyne/gestalt/commit/5cf16ca34d13be92b6be914e5f07bc7effd340f3))
* update gestalt-send session start ([c4df161](https://github.com/dyne/gestalt/commit/c4df1610b2c76f31d04903b4e34a9802e59ea0b2))
* wire flow config UI ([ea281ae](https://github.com/dyne/gestalt/commit/ea281ae278ffa05b7e3d2e26e5788d0e67029131))

# [1.2.0](https://github.com/dyne/gestalt/compare/v1.1.0...v1.2.0) (2026-01-28)


### Bug Fixes

* add logger propagation and middleware/rest handler logging ([a770014](https://github.com/dyne/gestalt/commit/a770014fc3d8282886442e3efc9cc2da583555e9))
* align history with cursor snapshot ([a03605b](https://github.com/dyne/gestalt/commit/a03605bbbaa6233c301c1bce6a480fff6723e579))
* downgrade resize event severity ([89f5a70](https://github.com/dyne/gestalt/commit/89f5a706bd24c2edd45efc90ec8a2a2d2291a91a))
* make session logging lossless ([565d225](https://github.com/dyne/gestalt/commit/565d225019dc62c4e00b5949e81db88c000d4b3f))


### Features

* add gestalt-notify cli ([0221303](https://github.com/dyne/gestalt/commit/022130324b3faee3a5542bc6298d8342062b2b7e))
* add history pagination cursor ([e8f1628](https://github.com/dyne/gestalt/commit/e8f1628b22873e0b68a02f574ac629fea271150c))
* add notify workflow signal ([05e4b9c](https://github.com/dyne/gestalt/commit/05e4b9c0316383a46c1cbb8fbca60c320bfaae98))
* add sse events stream ([3d453ed](https://github.com/dyne/gestalt/commit/3d453ed50d8db82f2bad5e229551b0d38c992eb3))
* add sse log stream ([6b005ef](https://github.com/dyne/gestalt/commit/6b005ef5c5c3cbbdb07334a76e9fe1258c1393d5))
* add sse store helper ([f6c255a](https://github.com/dyne/gestalt/commit/f6c255a96f7390e9c7db5021035524d08dea8174))
* add sse stream helpers ([968727e](https://github.com/dyne/gestalt/commit/968727e787b71478dc8833bc7e3265e522a5750d))
* add terminal history cursor ([d141b6b](https://github.com/dyne/gestalt/commit/d141b6b9c530da2269e2fe152afad9a945417b3d))
* add terminal notify endpoint ([751be23](https://github.com/dyne/gestalt/commit/751be23056f20ea9fd0e156cdd0d5ca87bac4029))
* add websocket catch-up for terminal logs ([89a246d](https://github.com/dyne/gestalt/commit/89a246d5a071fe007d228059b6e3a157c64f2ceb))
* align agent id in workflows ([fd32734](https://github.com/dyne/gestalt/commit/fd32734bc7dd6dabef3a67975e1f8fa3b1952b12))
* define notify envelope ([8ca50f5](https://github.com/dyne/gestalt/commit/8ca50f56c65e341023dd5fdcb97cae6b0c7d5960))
* inject codex notify default ([e2b7ff6](https://github.com/dyne/gestalt/commit/e2b7ff6224d9154ac1da5c1e73afdc0c50b1d66e))
* label notify history events ([40066c2](https://github.com/dyne/gestalt/commit/40066c20915111f1f0030fe9191ba0e43a01d124))
* migrate event stores to sse ([208910a](https://github.com/dyne/gestalt/commit/208910aa53daa92cc0c3f1ef21fa7098e96820a7))
* migrate log stream to sse ([ce9272b](https://github.com/dyne/gestalt/commit/ce9272b2e43e1e8ef14a9d55e2f9cc91dc98deb4))
* use cursor for terminal ws reconnect ([b7aee1e](https://github.com/dyne/gestalt/commit/b7aee1ef5b71533a75b7a509ca991effbc8e2675))

# [1.1.0](https://github.com/dyne/gestalt/compare/v1.0.0...v1.1.0) (2026-01-28)


### Bug Fixes

* coder now works with configs in toml ([8072c67](https://github.com/dyne/gestalt/commit/8072c67cb6f95b498ac8cbbc0835cc2ce6d09af0))
* command to launch gestalt in readme ([339d021](https://github.com/dyne/gestalt/commit/339d0211e7e2081151ec262d8b9555c0369cdc32))
* config prompts overlay and improve prompts ([fe2483b](https://github.com/dyne/gestalt/commit/fe2483b4d3cb55539cc609ee04e3830392711929))
* config settings on shell execution use k=v ([f575c3e](https://github.com/dyne/gestalt/commit/f575c3e7997e80f2d50288256c08c600d1e1eef0))
* elide fenced language tag in docs ([56e00b7](https://github.com/dyne/gestalt/commit/56e00b79562e69ec4b6fa48c9992d35e3ababa26)), closes [#B](https://github.com/dyne/gestalt/issues/B)
* focus close dialog programmatically ([752e258](https://github.com/dyne/gestalt/commit/752e258461c60ec3b52bec8002a32d2403323126))
* **frontend:** hide voice input on insecure contexts ([21063e0](https://github.com/dyne/gestalt/commit/21063e0a43f45b50f52a59f572ede2bfa38e21c6))
* improve graphical style ([d24d626](https://github.com/dyne/gestalt/commit/d24d626b7a20f2e29768e744ec21e5dce53725c6))
* improve palette and light/dark theme ([cbe3c92](https://github.com/dyne/gestalt/commit/cbe3c92b3e37690d8c07bec0db5c4b251a55a273))
* improve terminal layout on resize ([205cdcd](https://github.com/dyne/gestalt/commit/205cdcd356377ee49882f3461260071f02c02300))
* keep temporal client reference ([6bc0f2c](https://github.com/dyne/gestalt/commit/6bc0f2ca1ff5d615c240b0c30625327e4fd9b773))
* load header logos via imports ([a33ceba](https://github.com/dyne/gestalt/commit/a33ceba2d98275c9e89eb750c0cf8bc5801bff92))
* made AGENTS.md more compact ([cb44327](https://github.com/dyne/gestalt/commit/cb443272c45b76e8395ec74c6ab9d8e95cff1da5))
* make terminal views responsive ([8840cd9](https://github.com/dyne/gestalt/commit/8840cd9a8021226dd4dfdd6a39d41ecf64b40af6))
* missing typescript compiler ([3faa3dc](https://github.com/dyne/gestalt/commit/3faa3dc09603bad9f35d5d331835124b39a20c1c))
* omit unspecified kind in TOON output ([7860485](https://github.com/dyne/gestalt/commit/78604855804bfccbdfe4b706d0d5f47bfdfde89a)), closes [#B](https://github.com/dyne/gestalt/issues/B)
* remove busy-wait in PTY read loop ([01b64ce](https://github.com/dyne/gestalt/commit/01b64cedeca5ed3ed30f2b35ee8858f17d2aaf32))
* several fixes for concurrent run ([8bda8c1](https://github.com/dyne/gestalt/commit/8bda8c15a986c7788c7f57452d95f841a5bbee42))
* stabilize keyed lists ([8ca95e5](https://github.com/dyne/gestalt/commit/8ca95e5206f316bd863c429553d42929f67e43e5))
* switch to dotted sections in agent toml ([a260771](https://github.com/dyne/gestalt/commit/a2607718e409777bddb5a671eecf2d72bc450d24))
* sync running agent state ([9caaf23](https://github.com/dyne/gestalt/commit/9caaf23f06de5932ff6dba3d29c9c557dbecc5ea))


### Features

* add agent lifecycle events ([112bd51](https://github.com/dyne/gestalt/commit/112bd513491fd85d918d1afbaf8e32bfbd0fa399))
* add config change events ([9ba105a](https://github.com/dyne/gestalt/commit/9ba105a6a3440d58c85cc34a426987bb99b7ca18))
* add core event types ([35d6151](https://github.com/dyne/gestalt/commit/35d6151b1ec7408b2145f29d0d04ecc7fcd8d2af))
* add Dyne logo assets ([37bdd28](https://github.com/dyne/gestalt/commit/37bdd2802044139c434d93343a47d019e96a074a))
* add filtered event subscriptions ([9d4a188](https://github.com/dyne/gestalt/commit/9d4a1887fcea9cac6ba484b1a1d4cc27310f7891))
* add flow workflow view ([366ad30](https://github.com/dyne/gestalt/commit/366ad30133ab5bd4ca2cc406074c14c443c2295e))
* add generic event bus ([d38c221](https://github.com/dyne/gestalt/commit/d38c221af0a02e8717b3cd0ee07bfd18e98f99b4))
* add Gestalt typography logo ([78dc203](https://github.com/dyne/gestalt/commit/78dc20349ca79d598a904809c23c6d3b09906f6a))
* add make dev ([7deb667](https://github.com/dyne/gestalt/commit/7deb667f02dbdc3054c5ba8a670b729403a0ccdf))
* add plan current parsing ([4d4c856](https://github.com/dyne/gestalt/commit/4d4c856ca71cafd6520195ae6139ae21c7c5309e))
* add port registry resolver ([e0fcc2e](https://github.com/dyne/gestalt/commit/e0fcc2e813f6581ccc935e60474d33298424b2e1))
* add port resolver to prompt parser ([f3fb084](https://github.com/dyne/gestalt/commit/f3fb08419a1136496551a16a51aa63e90850ec48))
* add prompt separator after skills ([31b34f8](https://github.com/dyne/gestalt/commit/31b34f8bbdae531e52d31d7565c1ed42947d0020))
* add prompt template parser ([ac62ae8](https://github.com/dyne/gestalt/commit/ac62ae83d9a12e1d0bd6a3b295b509815fe4d680))
* add relative time formatting ([f13f1bb](https://github.com/dyne/gestalt/commit/f13f1bbd9c4f1ae9ff39dc35a068a0bf6be5bafa))
* add service discovery prompt example ([a1e94f1](https://github.com/dyne/gestalt/commit/a1e94f10ab6855cbe26fbf4dd4672311aaba508c))
* add temporal dev setup ([cffcd53](https://github.com/dyne/gestalt/commit/cffcd53668511e5f8525184be67087993d44a657))
* add temporal sdk client ([cc10cc7](https://github.com/dyne/gestalt/commit/cc10cc7d22e687f4a7dfb3da7141499cc97bdc75))
* add temporal session activities ([0f3efe5](https://github.com/dyne/gestalt/commit/0f3efe590284b7defc2f8a7bf643113d79dc9630))
* add temporal session workflow ([251a32a](https://github.com/dyne/gestalt/commit/251a32a40731fee3216812861eb33ca311250499))
* add temporal worker runner ([d030fdd](https://github.com/dyne/gestalt/commit/d030fddd41aa5edc85329a2ae08dabc41c92d61e))
* add terminal close confirmation ([7a9befd](https://github.com/dyne/gestalt/commit/7a9befd534e6a197372f44fe5b4f17ef3ddbc7cf))
* add terminal lifecycle events ([a523370](https://github.com/dyne/gestalt/commit/a5233701399233c1f6b4d16f503dfd313d06ecb3))
* add TOON output format to gestalt-scip ([bd5bd7b](https://github.com/dyne/gestalt/commit/bd5bd7b35ddb21b3fd95bc177b69f4eb2da2afc8)), closes [#B](https://github.com/dyne/gestalt/issues/B)
* add ui crash capture and overlay ([11c2da8](https://github.com/dyne/gestalt/commit/11c2da875cbf41cf8c40705e035d193bac90030c))
* add view error boundaries ([f97d3df](https://github.com/dyne/gestalt/commit/f97d3df50f64275a6c130879aca000f5cebd426e))
* add VoiceInput speech recognition component ([e1c1120](https://github.com/dyne/gestalt/commit/e1c1120a087bf1c7f943b9f2a929bc487e85bc77))
* add workflow detail components ([3969c35](https://github.com/dyne/gestalt/commit/3969c35ae49b1f2929e8704043032aa6c0e9f377))
* add workflow history API ([c68dc36](https://github.com/dyne/gestalt/commit/c68dc366deeb85e6fed33f4affa2eee372d90882))
* add workflow metrics ([db7566b](https://github.com/dyne/gestalt/commit/db7566bfbca9abbb727e1641fbb75752235c9733))
* add workflow resume actions ([bfb2110](https://github.com/dyne/gestalt/commit/bfb2110b5efea804562f8bd3195a527078135cde))
* add workflow-managed session creation ([fd8218e](https://github.com/dyne/gestalt/commit/fd8218e722727b7563c242e5de9b4cc229559cd1))
* adjust agent launch actions ([09479f7](https://github.com/dyne/gestalt/commit/09479f7aaa27550aaa1d4c9700c93c0517e8afbf))
* adjust prompt include priority ([9e52a26](https://github.com/dyne/gestalt/commit/9e52a2624b1b111394831f26b127d5e13b020c49))
* **api:** consolidate logs under /api/otel ([5d96531](https://github.com/dyne/gestalt/commit/5d96531c8cf626d0d3176c2d347d14027df7a77e))
* **api:** stream OTLP logs over ws ([c29d9f3](https://github.com/dyne/gestalt/commit/c29d9f361e2544f7f11c0beedb82d1ebeb624e3e))
* apply prompt directives to all prompt files ([0880647](https://github.com/dyne/gestalt/commit/08806471f0bee5de8a4e81d6ce83bc04a4d32257))
* auto-manage temporal dev server ([bd6addc](https://github.com/dyne/gestalt/commit/bd6addc14090ab7be83480f6d1f3eb78216e4d75))
* base64url encode scip symbol ids ([8422b83](https://github.com/dyne/gestalt/commit/8422b8367f43ac1512fe5593ccfbdf7b2e15daa1)), closes [#A](https://github.com/dyne/gestalt/issues/A)
* brand header with Dyne logos ([a5ddd57](https://github.com/dyne/gestalt/commit/a5ddd57516e58f572d21a3b73cf1be2b1b2688a9))
* configure workflow timeouts ([67a85a2](https://github.com/dyne/gestalt/commit/67a85a2920528ac84e4cd5f683eddfc8bcc1e772))
* dedupe prompt includes ([ec42f79](https://github.com/dyne/gestalt/commit/ec42f79fff6e7b712fca7512c4b8014b8eb59b3e))
* default workflow tracking on ([a8d6524](https://github.com/dyne/gestalt/commit/a8d6524e9d1ef94b7e7508caca2bbfe60ac0ddab))
* embed temporal server ([c6d4786](https://github.com/dyne/gestalt/commit/c6d4786dbf2a604558af6aa4f480399d506d0f96))
* enable temporal by default ([227047b](https://github.com/dyne/gestalt/commit/227047bd61f5ad6f2127fcb3d2992f4e6f868c91))
* enlarge header logotype ([6c72466](https://github.com/dyne/gestalt/commit/6c7246693ce9547dc90563b93f550cdd876e09ea))
* enrich prompt render logs ([ace746d](https://github.com/dyne/gestalt/commit/ace746d4a93eaa299a753b82b28ce6c34baf5436))
* extend prompt include lookup ([f41a24f](https://github.com/dyne/gestalt/commit/f41a24f3d3bdecdc29a2fd8c2d61469d213ecf5a))
* **frontend:** add plan cards view ([6479791](https://github.com/dyne/gestalt/commit/6479791dfd967c2a9909c5d0d8fa3041b5cac2d5))
* **frontend:** log UI actions via otel ingest ([6ca4b72](https://github.com/dyne/gestalt/commit/6ca4b72f2c1f6cbf6eba5697e6dfe97050165725))
* honor workdir include paths ([027a57c](https://github.com/dyne/gestalt/commit/027a57c1b2aaecbb1a11198d6be4242603d4f25e))
* improve voice input for mobile ([60edf86](https://github.com/dyne/gestalt/commit/60edf86b89825be5e7fcfcf8556201d95ad6d063))
* integrate prompt templates into sessions ([4a4ce3f](https://github.com/dyne/gestalt/commit/4a4ce3fda1081159601515350f6d54b0a2c4b29b))
* integrate sessions with temporal workflows ([3200a1c](https://github.com/dyne/gestalt/commit/3200a1c39e270485718638afe08878ed3775c7de))
* integrate voice input into command input ([f66fbd1](https://github.com/dyne/gestalt/commit/f66fbd1602c3b91461ecfef45e1ba07f2d03aea6))
* **logs:** add categories and correlations ([a8b7957](https://github.com/dyne/gestalt/commit/a8b79577a268eb4467a1ade210a4225af9184c90))
* make TOON the default scip format ([1071de8](https://github.com/dyne/gestalt/commit/1071de8c38ec73ba19050e087d7b28cd8224bbe2)), closes [#B](https://github.com/dyne/gestalt/issues/B)
* migrate log streaming bus ([d760958](https://github.com/dyne/gestalt/commit/d7609588922a5fc7d3abedb79280fa7edc14fce9)), closes [hi#volume](https://github.com/hi/issues/volume)
* migrate terminal output bus ([572dc9c](https://github.com/dyne/gestalt/commit/572dc9cf0432131f62620e2b94c3c64270a8257e))
* migrate watcher event hub ([41d98ca](https://github.com/dyne/gestalt/commit/41d98cae44c8ab302f14b90d9440f97c066b4c6e))
* new modular prompt structure ([2f0c19f](https://github.com/dyne/gestalt/commit/2f0c19f04397b6ad696c38e81a79112452114eba))
* normalize api client payloads ([5629434](https://github.com/dyne/gestalt/commit/5629434ad4adbf7f6cf1b056108c07177e7778e6))
* otel builder setup for otelcol-gestalt ([c381a73](https://github.com/dyne/gestalt/commit/c381a7344cf38a3611ea55bfc96aa7c82b7d06eb))
* **otel:** add log hub for log replay ([90a11c6](https://github.com/dyne/gestalt/commit/90a11c6bc73f653071e052ecb26f4f1983d23d8b))
* **otel:** add tail log reader ([17c4f9d](https://github.com/dyne/gestalt/commit/17c4f9d101280350f9b6f19ac6d7396c1c93d4a9))
* **otel:** lock UI logs to OTLP contract ([48f5af1](https://github.com/dyne/gestalt/commit/48f5af16c4a0b44dc4caaf3d644bf7a56b28f02a))
* **otel:** rotate collector log file ([7069a14](https://github.com/dyne/gestalt/commit/7069a14537ffe08af101ee523efdb983beaf6dad))
* **otel:** tap logs into log hub ([171b6b6](https://github.com/dyne/gestalt/commit/171b6b6c55250b6cfbfc5f8dcd392480ae6dee51))
* **plan:** add orga parser bridge ([045811d](https://github.com/dyne/gestalt/commit/045811dbabbbb2cb42101ffea9a484dfe744c01f))
* **plan:** add plans list endpoint ([fc8fa00](https://github.com/dyne/gestalt/commit/fc8fa009df1610538f1cd383b72f05c94cb38416))
* **plans:** watch plans directory ([0dc1c0c](https://github.com/dyne/gestalt/commit/0dc1c0cc9971dc00eede40ee198c77c2f13583f3))
* profiling mode for go pprof ([794c461](https://github.com/dyne/gestalt/commit/794c461527b2715937faf4a27503357746ff7ed7))
* refine prompt include lookup ([028b18c](https://github.com/dyne/gestalt/commit/028b18c47a631045f9ac4c23ed587b10db0edcba))
* register runtime service ports ([d5a3629](https://github.com/dyne/gestalt/commit/d5a3629479948be5555d4486a448787e4b472a93))
* relocate terminal close control ([1868e76](https://github.com/dyne/gestalt/commit/1868e76090f2e255546e83c23f53feeec7acabee))
* remove terminal tab close buttons ([0193cdb](https://github.com/dyne/gestalt/commit/0193cdbca2e1cded8c1454a7c0226006b4b507fb))
* rename Flow tab label ([497e498](https://github.com/dyne/gestalt/commit/497e498ebc54f372404bdb1de9be1240d9bb790c))
* resolve {{port}} directives in prompts ([fcde717](https://github.com/dyne/gestalt/commit/fcde717e606a4d2ef88b656ff6605342e051a4d3))
* resolve prompt includes from workdir ([ef3761c](https://github.com/dyne/gestalt/commit/ef3761c3de41e95834ca05ed511ffe5a3f74209e))
* restore running-state styling ([246681f](https://github.com/dyne/gestalt/commit/246681ffaf058cf156bf5015f7b29e46f65b3257))
* SCIP fist implementation with sqlite ([a7c32fd](https://github.com/dyne/gestalt/commit/a7c32fdb1ef03e09c1ec1533cc4758585be7f381))
* **scip-cli:** add definition and references ([a3b40be](https://github.com/dyne/gestalt/commit/a3b40bec560aebb716fd1a20e00066bc32ee6872))
* **scip-cli:** add reusable SCIP parsing library ([cfe720f](https://github.com/dyne/gestalt/commit/cfe720f959bfc0c0b68025f140f5f00f742e7896))
* **scip-cli:** add SCIP file discovery ([8a53d6f](https://github.com/dyne/gestalt/commit/8a53d6fcd74dd1cd77941ef238628895cc16c42d))
* **scip-cli:** implement files command ([0fca179](https://github.com/dyne/gestalt/commit/0fca179a312a6a8afec6adfeae177db651eac4a6))
* **scip-cli:** implement symbols search ([cd547ac](https://github.com/dyne/gestalt/commit/cd547ac10fecd380ea7dabdb387f296b39f7beb5))
* **scip-cli:** wire commander CLI ([5c7b6ab](https://github.com/dyne/gestalt/commit/5c7b6ab8703609e5d733abd22c6fb527c81b3d87))
* **scip:** add async indexing status and dashboard indicator ([3bb35f5](https://github.com/dyne/gestalt/commit/3bb35f51f3a47043de05855623cca34e78fc238f))
* show listening indicator during voice input ([b5d715c](https://github.com/dyne/gestalt/commit/b5d715cf463ecc9acf15dcbf223c8f29831eb703))
* show prompt files in terminal header ([e7f58b7](https://github.com/dyne/gestalt/commit/e7f58b7202b413b79bf365c7f394940e7a55eb4e))
* swap brand text for logo ([c97f294](https://github.com/dyne/gestalt/commit/c97f29489b26d4a1ced6334967d704a2f5c9a5ca))
* **ui:** render OTLP log records ([ad349ad](https://github.com/dyne/gestalt/commit/ad349addac8f4720f73a3e073bdbeb332ae78a84))
* use relative timestamps in UI ([921d47e](https://github.com/dyne/gestalt/commit/921d47e484ca81b88eba9e663284e53c01785d0e))
* wire port resolver into prompt rendering ([a146efc](https://github.com/dyne/gestalt/commit/a146efc96ed87c6a7969fc05d6294987d00fbd75))
* wire terminal bell to workflows ([f3c9130](https://github.com/dyne/gestalt/commit/f3c91303a7af3878862971264ba7ad08c1cd3b33))


### Performance Improvements

* batch log updates ([0cbd3a8](https://github.com/dyne/gestalt/commit/0cbd3a84df5ba64f49cb365336d292eab4497510))


### Reverts

* Revert "feat: embed temporal server" ([e797b4e](https://github.com/dyne/gestalt/commit/e797b4edbc0c7cd0c2abe0ece59d3db8247dd144))
