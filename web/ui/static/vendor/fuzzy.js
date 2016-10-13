// Copyright (c) 2012 Matt York
//
// Permission is hereby granted, free of charge, to any person
// obtaining a copy of this software and associated documentation
// files (the "Software"), to deal in the Software without
// restriction, including without limitation the rights to use,
// copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following
// conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
// HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.
//
// https://github.com/mattyork/fuzzy/blob/3613086aa40c180ca722aeaf48cef575dc57eb5d/fuzzy-min.js

(function(){var root=this;var fuzzy={};if(typeof exports!=="undefined"){module.exports=fuzzy}else{root.fuzzy=fuzzy}fuzzy.simpleFilter=function(pattern,array){return array.filter(function(str){return fuzzy.test(pattern,str)})};fuzzy.test=function(pattern,str){return fuzzy.match(pattern,str)!==null};fuzzy.match=function(pattern,str,opts){opts=opts||{};var patternIdx=0,result=[],len=str.length,totalScore=0,currScore=0,pre=opts.pre||"",post=opts.post||"",compareString=opts.caseSensitive&&str||str.toLowerCase(),ch;pattern=opts.caseSensitive&&pattern||pattern.toLowerCase();for(var idx=0;idx<len;idx++){ch=str[idx];if(compareString[idx]===pattern[patternIdx]){ch=pre+ch+post;patternIdx+=1;currScore+=1+currScore}else{currScore=0}totalScore+=currScore;result[result.length]=ch}if(patternIdx===pattern.length){totalScore=compareString===pattern?Infinity:totalScore;return{rendered:result.join(""),score:totalScore}}return null};fuzzy.filter=function(pattern,arr,opts){if(!arr||arr.length===0){return[]}if(typeof pattern!=="string"){return arr}opts=opts||{};return arr.reduce(function(prev,element,idx,arr){var str=element;if(opts.extract){str=opts.extract(element)}var rendered=fuzzy.match(pattern,str,opts);if(rendered!=null){prev[prev.length]={string:rendered.rendered,score:rendered.score,index:idx,original:element}}return prev},[]).sort(function(a,b){var compare=b.score-a.score;if(compare)return compare;return a.index-b.index})}})();
