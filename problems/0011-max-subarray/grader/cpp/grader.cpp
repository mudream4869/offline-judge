#include <cstdio>
#include <vector>

long long max_subarray(const std::vector<int>& a);

int main() {
    int n;
    if (std::scanf("%d", &n) != 1) return 1;
    std::vector<int> a(n);
    for (int& x : a) std::scanf("%d", &x);
    std::printf("%lld\n", max_subarray(a));
}
