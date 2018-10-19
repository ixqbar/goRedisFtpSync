#!/usr/bin/python
# -*- coding: utf-8 -*-

import os
import redis

pool = redis.ConnectionPool(host='127.0.0.1', port=8399, db=0)
rd = redis.Redis(connection_pool=pool)

LOCAL_PATH="/data/cdn/images"

for r,d,f in os.walk(LOCAL_PATH):
    for ifg in f:
        t = os.path.join(r, ifg)
        start = 1 + len(LOCAL_PATH)
        print(os.path.join(r, ifg), t[start:])
        rd.execute_command("ftpasync", os.path.join(r, ifg), "/prd_asset/data/images/shop/" + t[start:])