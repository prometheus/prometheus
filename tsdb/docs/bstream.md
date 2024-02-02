# bstream details

This doc describes details of the bstream (bitstream) and how we use it for encoding and decoding.
This doc is incomplete.  For more background, see the Gorilla TSDB [white paper](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf)
or the original [go-tsz](https://github.com/dgryski/go-tsz) implementation, which this code is based on.

## Delta-of-delta encoding for timestamps

We need to be able to encode and decode dod's for timestamps, which can be positive, zero, or negative.
Note that int64's are implemented as [2's complement](https://en.wikipedia.org/wiki/Two%27s_complement)

and look like:

```
0111...111 = maxint64
    ...
0000...111 = 7
0000...110 = 6
0000...101 = 5
0000...100 = 4
0000...011 = 3
0000...010 = 2
0000...001 = 1
0000...000 = 0
1111...111 = -1
1111...110 = -2
1111...101 = -3
1111...100 = -4
1111...011 = -5
1111...010 = -6
1111...001 = -7
1111...000 = -8
    ...
1000...001 = minint64+1
1000...000 = minint64
```

All numbers have a prefix (of zeroes for positive numbers, of ones for negative numbers), followed by a number of significant digits at the end.
In all cases, the smaller the absolute value of the number, the fewer the amount of significant digits.

To encode these numbers, we use:
* A prefix which declares the amount of bits that follow (we use a predefined list of options in order of increasing number of significant bits).
* A number of bits which is one more than the number of significant bits.  The extra bit is needed because we deal with unsigned integers, although
  it isn't exactly a sign bit. (See below for details).

The `bitRange` function determines whether a given integer can be represented by a number of bits.
For a given number of bits `nbits` we can distinguish (and thus encode) any set of `2^nbits` numbers.
E.g. for `nbits = 3`, we can encode 8 distinct numbers, and we have a choice of choosing our boundaries.  For example -4 to 3,
-3 to 4, 0 to 7 or even -2 to 5 (always inclusive).  (Observe in the list above that this is always true.)
Because we need to support positive and negative numbers equally, we choose boundaries that grow symmetrically.  Following the same example,
we choose -3 to 4.

When decoding the number, the most interesting part is how to recognize whether a number is negative or positive, and thus which prefix to set.
Note that the bstream library doesn't interpret integers to a specific type, but rather returns them as uint64's (which are really just a container for 64 bits).
Within the ranges we choose, if looked at as unsigned integers, the higher portion of the range represent the negative numbers.
Continuing the same example, the numbers 001, 010, 011 and 100 are returned as unsigned integers 1,2,3,4 and mean the same thing when casted to int64's.
But the others, 101, 110 and 111 are returned as unsigned integers 5,6,7 but actually represent -3, -2 and -1 (see list above),
The cutoff value is the value set by the `nbit`'th bit, and needs a value subtracted that is represented by the `nbit+1`th bit.
In our example, the 3rd bit sets the number 4, and the 4th sets the number 8. So if we see an unsigned integer exceeding 4 (5,6,7) we subtract 8. This gives us our desired values (-3, -2 and -1).

Careful observers may note that, if we shift our boundaries down by one, the first bit would always indicate the sign (and imply the needed prefix).
In our example of `nbits = 3`, that would mean the range from -4 to 3.  But what we have now works just fine too.
