using System;
using Xunit;

public class Greeting_Should
{
    [Fact]
    public void Greeting_PrintName()
    {
	    var name = "Dagger Test";
	    var expect = "Hi Dagger Test!";
	    var value = Greeting.GetMessage(name);
        Assert.Equal(expect, value);
    }
}
