# Using locci

## First run

```sh
cp .env.sample .env
just setup                       # .env, Go modules, web packages (bun)
just all                         # build the UI, then the binary
./locci user add admin --admin   # prompts for a password
./locci serve                    # http://localhost:8998
```

Or run it in Docker: see [DOCKER.md](DOCKER.md).

Sign in at `http://localhost:8998`. The first page is the overview of the
default environment.

## The layout

- **Sidebar:** the environment switcher at the top (with an up/down light per
  host), the fleet pages for that environment, Environments, and for admins
  Users and Audit log. Your account menu (log out) is at the bottom.
- **Theme:** the sun/moon button switches between light, dark and the system
  setting.
- **Phones:** the sidebar becomes a menu behind the top-left button. Tables
  turn into cards.
- **Lists** are paginated, 10 rows by default. The rows-per-page choice is
  remembered in your browser.
- **Confirmations** guard everything that removes something. Results and
  errors appear as toasts at the bottom right.

## Overview

Counts of containers, images, networks and volumes, running versus stopped,
the five newest containers and the host's details (Docker version, CPUs,
memory, OS, storage driver).

## Containers

- Search by name, image or state, and filter by state.
- Each row has its main action (Start, Stop or Resume) and a menu with the
  rest: restart, pause, kill (on the container page) and remove.
- **New container** asks for a name and image, then optionally a subdomain,
  command, ports (`CONTAINER:HOST` per line), networks, environment
  variables, volumes and labels. A preview of the request is shown as you
  type. The image is pulled first; a local image is used if the pull fails.
- **A container's page** has tabs:
  - Overview: image, command, ports, networks, mounts, environment, labels.
  - Logs: live, with follow and clear.
  - Stats: CPU (with a short history), memory, network, processes; refreshes
    every 3 seconds.
  - Console: an interactive shell (`sh`, `bash` or `ash`) in the browser.
  - Inspect: the raw JSON, with a copy button.

Click any ID to copy it.

## Images, networks, volumes

Each page lists what's on the host. **Pull image**, **New network** and **New
volume** open a dialog. Images can be pruned (dangling only). The built-in
networks (`bridge`, `host`, `none`) can't be removed.

## Environments

Cards for every Docker host with its status, Docker version, running
containers, CPUs, memory and OS. Click one to switch to it. Admins can add,
edit, set the default, check now and remove. See
[ENVIRONMENTS.md](ENVIRONMENTS.md).

## Users (admin)

Add users with a password and the role `user` or `admin`, and delete them
(not yourself). Admins also see environments' admin actions, users and the
audit log.

## Audit log (admin)

Every change made through the API: when, who, the request and whether it
succeeded. Filter by outcome.

## From the command line

Everything above is also available from the CLI; see [CLI.md](CLI.md).
