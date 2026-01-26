# WORKPLAN

Do not use markdown TODOs.

Projects plans are Org-Mode files (.org extension) in .gestalt/plans

Filenames should just contain a few strings on their topic (no date)

File Contents:

1. L1 = first-level heading (*)
2. L2 = second-level heading (**)

Their headings are marked (XOR marks):
- implemented (DONE)
- to be done (TODO)
- being worked on (WIP).

Headings are sorted top-to-bottom in execution order.

Every L1 and L2 heading include BOTH:
 - a TODO keyword: TODO / WIP / DONE
 - a priority estimation: [#A] [#B] [#C]

Priority scale:
- [#A] major feature, important bug, large refactor
- [#B] default: normal feature/bugfix/small change
- [#C] polish, optimization, small refactor

## Template

#+TITLE: A Brief Plan Title
#+SUBTITLE: Longer one-liner description of the plan
#+DATE: date of creation
#+KEYWORDS: comma separated list of single words

* TODO [#B] Short plan title
  Effort: total nr of hours for all L1
  Goal: ...
  Notes: ...
** TODO [#C] Step title
   Why: ...
   Change: ...
   Tests: ...
   Done when: ...
