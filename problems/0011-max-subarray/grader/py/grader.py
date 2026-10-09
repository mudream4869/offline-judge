import sys

from solution import max_subarray

data = sys.stdin.buffer.read().split()
n = int(data[0])
print(max_subarray([int(x) for x in data[1:1 + n]]))
