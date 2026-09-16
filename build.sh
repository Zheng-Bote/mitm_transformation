#!/usr/bin/sh


cargo build --release --target x86_64-unknown-linux-musl
cp target/x86_64-unknown-linux-musl/release/mitm-transformer /home/zb_bamboo/DEV/__NEW__/Go/mitm-2/scheduler/mitm_scheduler/bin/.

