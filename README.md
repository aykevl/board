# TinyGo board abstraction

**This is an experiment.** It may go stale, or it may change in backwards incompatible ways. Don't rely on it too much for now.

## Goals

  * Provide a common abstraction for boards supported by TinyGo.
  * Provide a simulated board
  * Auto-generate all board definitions from a devicetree-like format. Don't write any of the definitions by hand. (See Zephyr OS for example).
  * Allow generating a similar board definition from target JSON files for custom boards.
  * Make it possible to identify hardware at compile time instead of relying on build tags, as long as this doesn't impact binary size too much.

## Non-goals

  * Provide access to hardware unique to a particular board. A new interface has to be reasonably common, and has to be supported by at least two different boards.
  * Support custom boards from this package. Only boards that are (or were) actually produced should be supported. This includes dev boards, maker boards, electronic badges, etc.

## License

BSD 2-clause license, see LICENSE.txt for details.
