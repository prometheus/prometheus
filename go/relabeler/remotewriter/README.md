1. Create cursor.
2. Create client.
3. Loop:
    1. create batch. (readTimeout)
        1. read next segment.
            1. if permanent error - batch completed + end of block is reached.
            2. if not permanent error - if batch is fulfilled or deadline reached - batch is completed.
                1. recalculate number of output shards.
            3. if no error, and batch is not full, wait for 5 sec and repeat
    2. try go (write cache). (retry+backoff)
    3. encode protobuf.
    4. send. (retry+backoff)
        1. if outdated
            1. return permanent error
        2. on error -> non permanent error
        3. on success -> nil
    5. try ack.
    6. check end of block
   