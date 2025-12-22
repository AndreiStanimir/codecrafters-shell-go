package main

type Trie struct {
	next   map[rune]*Trie
	isWord bool
}

func Constructor() Trie {
	return Trie{
		next: make(map[rune]*Trie),
	}
}

func (this *Trie) Insert(word string) {
	node := this
	for _, ch := range word {
		if node.next[ch] == nil {
			node.next[ch] = &Trie{
				next: make(map[rune]*Trie),
			}
		}
		node = node.next[ch]
	}
	node.isWord = true
}

func (this *Trie) Search(word string) bool {
	if len(word) <= 0 {
		if this.isWord {
			return true
		}
		return false
	}
	if len(this.next) == 0 {
		return false
	}
	if next, ok := this.next[rune(word[0])]; ok {
		return next.Search(word[1:])
	}
	return false
}

func (this *Trie) StartsWith(prefix string) bool {
	if len(prefix) <= 0 {
		return true
	}
	if len(this.next) == 0 {
		return false
	}
	if next, ok := this.next[rune(prefix[0])]; ok {
		return next.StartsWith(prefix[1:])
	}
	return false
}
